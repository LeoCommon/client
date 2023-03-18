package handler

// This defines a generic handler that manages jobs

import (
	"runtime"
	"sync"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/backend"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/internal/client/task/scheduler"
	"disco.cs.uni-kl.de/apogee/pkg/log"
)

type TaskHandler struct {
	sync.RWMutex
	backend   backend.Backend
	scheduler *scheduler.Scheduler
	app       *client.App
}

func (h *TaskHandler) Shutdown() {
	h.scheduler.Shutdown()
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
	return h.scheduler.Cancel(id)
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
		handlerFunc, exclusiveResources := h.backend.GetJobHandlerFromParameters(params)
		if handlerFunc == nil {
			log.Error("no handler for job", zap.String("job", job.Json()))
			h.MarkFailed(job)
			continue
		}

		// Create a new task object
		task := scheduler.
			NewTask(job.StartTime, job.EndTime, handlerFunc, params).
			WithResource(exclusiveResources...).WithID(job.Id)

		// Schedule it
		err := h.scheduler.Schedule(task)
		if err != nil {
			// Unstarted tasks are allowed to be updated, so dont error out on AlreadyExists
			// And if the identical task is already running, we also dont do anything
			if err == scheduler.ErrTaskAlreadyExists || err == scheduler.ErrTaskAlreadyRunning {
				continue
			}

			log.Error("could not schedule job", zap.Error(err))
			h.MarkFailed(job)
			continue
		}

		// Output some info about the job
		log.Info("scheduled new job", zap.String("job", job.Json()))
	}

	return nil
}

// Returns true if any job is currently running
func (h *TaskHandler) HasRunningJob() bool {
	return h.scheduler.HasRunningJob()
}

func NewJobHandler(app *client.App) (*TaskHandler, error) {
	jh := &TaskHandler{}
	jh.app = app

	// Set up the rest api backend
	backend, err := backend.NewRestAPIBackend(app.Api)
	jh.backend = backend

	// Set up scheduler with NPROC workers
	jh.scheduler = scheduler.NewScheduler(runtime.NumCPU())

	// We can launch the go-routing here as we tear-down in .Shutdown()
	go jh.scheduler.Run()

	return jh, err
}
