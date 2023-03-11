package handler

// This defines a generic handler that manages jobs

import (
	"time"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/task/backend"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/pkg/log"
)

type JobHandler struct {
	scheduler *gocron.Scheduler
	app       *client.App
	backend   backend.Backend
}

// some constants
const maxJobDuration = 24 * 3600 // seconds

func (h *JobHandler) Shutdown() {
	h.scheduler.Clear()
}

// Performs a checkin operation with the server
func (h *JobHandler) Checkin() error {
	status, err := jobs.GetDefaultSensorStatus(h.app)
	if err != nil {
		return err
	}

	// Try to "check-in" with the server
	return h.app.Api.PutSensorUpdate(status)
}

func (h *JobHandler) Tick() {
	log.Debug("Polling jobs.")
	newJobs, err := h.app.Api.GetJobs()

	if err != nil {
		log.Error("Failed to fetch jobs, sitting this one out")
		return
	}

	scheduledJobs := []string{}
	rescheduledJobs := []string{}
	for _, job := range newJobs {
		// Schedule the job here
		params := &backend.JobParameters{}
		params.Job = job
		params.App = h.app
		rescheduleDetected := false

		handlerFunc := h.backend.GetJobHandlerFromParameters(params)

		if handlerFunc == nil {
			log.Error("No handler found for job with parameters", zap.Any("job", job))
			continue
		}

		// If job is already scheduled, remove it and reschedule (necessary to avoid missing jobs with overlapped start)
		list, _ := h.scheduler.FindJobsByTag(job.Id) // Ignore the error of this function it's not really an "error"
		if len(list) > 0 {
			removeError := h.scheduler.RemoveByTag(job.Id)
			if removeError != nil {
				log.Error("Unable to reschedule job, maybe it still works", zap.String("oldJob", job.Name), zap.Error(removeError))
			}
			rescheduleDetected = true
		}

		// If the job is expired (job.EndTime < time.Now) don't schedule it
		if time.Now().Unix() > job.EndTime {
			// If the job is older than 60 sec, send a 'failed' job-status is sent to the server
			if time.Now().Unix()-job.EndTime > 60 {
				log.Debug("Expired old job found: send 'failed' status", zap.Any("oldJob", job.Name))
				err = h.app.Api.PutJobUpdate(job.Name, "failed")
				if err != nil {
					log.Error("Unable to send 'failed' status to expired job", zap.String("oldJob", job.Name))
				}
			} else {
				// Don't send a failed-status directly, maybe it is still in the finishing process
				log.Debug("Expired job found: wait if it removes itself", zap.Any("oldJob", job.Name))
			}
			continue
		}

		// Check if the endTime of the task is proper set (after startTime & max 24h long)
		if job.StartTime > job.EndTime || (job.EndTime-job.StartTime) > maxJobDuration {
			log.Error("Invalid job details: Job potentially running too long", zap.String("job", job.Name))
			err = h.app.Api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				log.Error("Unable to send 'failed' status of too long running job", zap.String("job", job.Name), zap.NamedError("statusError", err))
			}
			continue
		}

		// For now only single shot tasks are supported
		// todo: as we can only run one task at a time, figure out some "timeout" mechanism that terminates stuck jobs
		_, err := h.scheduler.Tag(job.Id).Every(1).Millisecond().LimitRunsTo(1).StartAt(time.Unix(job.StartTime, 0)).DoWithJobDetails(handlerFunc, params)
		if err != nil {
			log.Error("Error during scheduling job", zap.String("job", job.Name), zap.NamedError("schedulingError", err))
			err = h.app.Api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				log.Error("Unable to send 'failed' status after errored job scheduling", zap.String("job", job.Name), zap.NamedError("statusError", err))
			}
		}
		if rescheduleDetected {
			rescheduledJobs = append(rescheduledJobs, job.Name)
		} else {
			scheduledJobs = append(scheduledJobs, job.Name)
		}
	}
	if len(scheduledJobs) > 0 {
		log.Debug(" new scheduled jobs", zap.Any("scheduledList", scheduledJobs))
	}
	if len(rescheduledJobs) > 0 {
		log.Debug(" rescheduled jobs", zap.Any("rescheduledList", rescheduledJobs))
	}
}

// Returns true if a job is currently running
func (h *JobHandler) HasRunningJob() bool {
	for _, job := range h.scheduler.Jobs() {
		if job.IsRunning() {
			return true
		}
	}

	return false
}

func NewJobHandler(app *client.App) (*JobHandler, error) {
	jh := &JobHandler{}
	jh.app = app

	// Set up the rest api backend
	backend, err := backend.NewRestAPIBackend(app.Api)
	jh.backend = backend

	jh.scheduler = gocron.NewScheduler(time.UTC)
	// Force 1 concurrent job, and reschedule if not possible
	// As we use one-off tasks this does not re-schedule so we rely on our server returning the job on the next poll!
	jh.scheduler.SetMaxConcurrentJobs(1, gocron.RescheduleMode)
	jh.scheduler.StartAsync()

	return jh, err
}
