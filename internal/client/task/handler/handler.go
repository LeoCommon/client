package handler

// This defines a generic handler that manages jobs

import (
	"context"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/backend"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/pkg/log"
)

type JobCanceller struct {
	job    *gocron.Job
	cancel context.CancelFunc
}

type TaskHandler struct {
	sync.RWMutex
	backend   backend.Backend
	scheduler *gocron.Scheduler
	app       *client.App

	// Map index is the unique job id
	cancellers map[string]JobCanceller
}

func (h *TaskHandler) Shutdown() {
	h.Lock()
	defer h.Unlock()
	h.scheduler.Clear()
}

// Performs a checkin operation with the server
func (h *TaskHandler) Checkin() error {
	status, err := jobs.GetDefaultSensorStatus(h.app)
	if err != nil {
		return err
	}

	// Try to "check-in" with the server
	return h.app.Api.PutSensorUpdate(status)
}

// Asynchronously mark a job as failed
func (h *TaskHandler) MarkFailed(job api.FixedJob) {
	go h.app.Api.PutJobUpdate(job.Name, "failed")
}

func VerifyBasicJobFacts(job api.FixedJob) bool {
	// fixme As long as we use a name as identifier in our code, all jobs need a name
	if len(job.Name) == 0 || len(job.Id) == 0 {
		log.Error("empty job name/id not permitted", zap.String("job", job.Json()))
		return false
	}

	// If the start time is not <= end time dont schedule it
	if job.StartTime.After(job.EndTime) {
		log.Error("invalid job start/end time", zap.String("job", job.Json()))
		return false
	}

	// Check if the job exceeds the maximum job duration
	if job.EndTime.Sub(job.StartTime) >= MaxJobDuration {
		log.Error("job duration exceeds the maximum", zap.Duration("max_sec", MaxJobDuration), zap.String("job", job.Json()))
		return false
	}

	return true
}

// CancelJob cancels the job with the given ID.
// Returns true if the job was found and cancelled, false otherwise.
func (h *TaskHandler) CancelJob(id string) bool {
	h.Lock()
	defer h.Unlock()

	if cancelContext, ok := h.cancellers[id]; ok {
		// Remove the job by reference
		h.scheduler.RemoveByReference(cancelContext.job)
		return h.cancelJobInternal(id, cancelContext.cancel)
	}

	return false
}

// cancelJobInternal carries out the handler internal cancel operations
func (h *TaskHandler) cancelJobInternal(id string, cancel context.CancelFunc) bool {
	// This will signal any goroutines using this context to stop running
	cancel()

	// Remove the canceller from the map
	delete(h.cancellers, id)
	return true
}

func (h *TaskHandler) Tick() error {
	log.Debug("Polling jobs.")
	newJobs, err := h.app.Api.GetJobs()

	if err != nil {
		log.Error("Failed to fetch jobs, sitting this one out")
		return err
	}

	for _, job := range newJobs {
		params := &backend.JobParameters{}
		params.Job = job
		params.App = h.app

		// Check some basic job facts
		if !VerifyBasicJobFacts(job) {
			h.MarkFailed(job)
			continue
		}

		// If no job handler was found, mark as failed and continue
		handlerFunc, jobType := h.backend.GetJobHandlerFromParameters(params)
		if handlerFunc == nil {
			log.Error("no handler for job", zap.String("job", job.Json()))
			h.MarkFailed(job)
			continue
		}

		// We always tag the unique job identifier too, so if we find it in the list, we already scheduled it once
		if j := h.GetJobWithUniqueTag(job.Id); j != nil {
			// If that job is already (close to) running, prevent re-scheduling
			if j.IsRunning() || time.Now().UTC().Sub(j.ScheduledTime()) < EditDeadline {
				log.Debug("skipping re-schedule of already (close to) running job", zap.Bool("running", j.IsRunning()), zap.String("job", job.Json()))
				// todo: check if the job needs to be cancelled
				// <MartB> Cancelling a job works by invoking h.CancelJob()
				h.Unlock()
				continue
			}

			// Remove the job so we dont miss the case where parameters might have been changed
			log.Debug("re-scheduling job", zap.String("job", job.Json()))
			h.Lock()
			h.scheduler.RemoveByReference(j)
			h.Unlock()
		}

		// If the job already expired, we are not allowed to schedule it
		// This "mark failed" wont affect already running jobs as they get handled above
		if time.Now().UTC().After(job.EndTime) {
			log.Warn("refusing to add expired job", zap.String("job", job.Json()))
			h.MarkFailed(job)
			continue
		}

		// Grab a list of singleton jobs, so we can get their real execution times
		singletonJobs := h.GetJobsWithTag(string(backend.JobTypeSingleton))
		for _, v := range singletonJobs {
			// fixme: no way to get the end_time, where do we store this?
			// <MartB> i ran into multiple limitations with go-cron already, might be wise to write our own
			// We only need a simple version without cron capabilities
			log.Error("stub: no special handling for singleton job type", zap.Strings("tags", v.Tags()))
		}

		// Create a new job canceller
		jobCanceller := JobCanceller{}
		// Assign the newly created cancellable context to the job params and save the cancel func
		params.Ctx, jobCanceller.cancel = context.WithCancel(context.Background())

		// Start adding the job
		h.Lock()
		// Add the unique ID as a tag and the job type (e.g Default or Singleton)
		goCronJob, err := h.scheduler.Tag(job.Id, string(jobType)).
			Every(1).
			Millisecond().
			LimitRunsTo(1).
			StartAt(job.StartTime).
			DoWithJobDetails(handlerFunc, params)
		h.Unlock()

		// If an error occured while trying to add the job
		if err != nil {
			log.Error("error during job scheduling", zap.String("job", job.Json()), zap.Error(err))
			h.MarkFailed(job)
			jobCanceller.cancel()
			continue
		}

		// Add a job reference to our canceller
		jobCanceller.job = goCronJob

		// Add the canceller to our list
		h.Lock()
		h.cancellers[job.Id] = jobCanceller
		h.Unlock()

		// Register the required event handlers to "clean up" the job
		goCronJob.SetEventListeners(func() {}, func() {
			// Runs after the job completes
			h.cancelJobInternal(job.Id, jobCanceller.cancel)
		})
	}

	return nil
}

// Returns the list of jobs that match a single tag
func (h *TaskHandler) GetJobsWithTag(tag string) []*gocron.Job {
	return h.MatchJobs(func(j *gocron.Job) bool {
		return slices.Contains(j.Tags(), tag)
	})
}

// Returns true if a job with the tag already exists
func (h *TaskHandler) HasJobWithTag(tag string) bool {
	return h.GetJobWithUniqueTag(tag) != nil
}

// Returns a job with a specific job, make sure you do use unique tags!
func (h *TaskHandler) GetJobWithUniqueTag(tag string) *gocron.Job {
	return h.MatchJob(func(j *gocron.Job) bool {
		return slices.Contains(j.Tags(), tag)
	})
}

// Returns true if any job is currently running
func (h *TaskHandler) HasRunningJob() bool {
	return h.MatchJob(func(j *gocron.Job) bool {
		return j.IsRunning()
	}) != nil
}

// MatchJob takes a matcher function as parameter and checks if the job list contains
// a job where the matcher evaluates to true, if it does it returns the job, else nil
func (h *TaskHandler) MatchJob(matcher func(j *gocron.Job) bool) *gocron.Job {
	h.RLock()
	defer h.RUnlock()

	for _, job := range h.scheduler.Jobs() {
		if matcher(job) {
			return job
		}
	}

	return nil
}

// MatchJobs takes a matcher function as parameter and checks if the job list contains
// jobs where the matcher evaluates to true and returns the slice of matching jobs
func (h *TaskHandler) MatchJobs(matcher func(j *gocron.Job) bool) []*gocron.Job {
	h.RLock()
	defer h.RUnlock()

	var matchedJobs []*gocron.Job
	for _, job := range h.scheduler.Jobs() {
		if matcher(job) {
			matchedJobs = append(matchedJobs, job)
		}
	}

	return matchedJobs
}

func NewJobHandler(app *client.App) (*TaskHandler, error) {
	jh := &TaskHandler{}
	jh.app = app

	// Create the map
	jh.cancellers = make(map[string]JobCanceller)

	// Set up the rest api backend
	backend, err := backend.NewRestAPIBackend(app.Api)
	jh.backend = backend

	jh.scheduler = gocron.NewScheduler(time.UTC)
	// Force unique tags directly on the scheduler
	// fixme Remove this line once we support running multiple at the same time
	jh.scheduler.SetMaxConcurrentJobs(1, gocron.WaitMode)
	jh.scheduler.StartAsync()

	return jh, err
}
