package handler

// This defines a generic handler that manages jobs

import (
	"time"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/task/backend"
	"disco.cs.uni-kl.de/apogee/pkg/task/jobs"
)

type JobHandler struct {
	scheduler *gocron.Scheduler
	app       *apogee.App
	backend   backend.Backend

	jobList []gocron.Job
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
	err = api.PutSensorUpdate(status)
	return err
}

func (h *JobHandler) Tick() {
	apglog.Debug("Polling jobs.")
	newJobs, err := api.GetJobs()

	if err != nil {
		apglog.Error("Failed to fetch jobs, sitting this one out")
		return
	}

	var skippedJobs []string
	for _, job := range newJobs {
		// Schedule the job here
		params := &backend.JobParameters{}
		params.Job = job
		params.App = h.app

		handlerFunc := h.backend.GetJobHandlerFromParameters(params)

		if handlerFunc == nil {
			apglog.Error("No handler found for job with parameters", zap.Any("job", job))
			continue
		}

		// Ignore jobs that are already scheduled
		list, _ := h.scheduler.FindJobsByTag(job.Id) // Ignore the error of this function it's not really an "error"
		if len(list) > 0 {
			skippedJobs = append(skippedJobs, job.Name)
			//apglog.Debug("Skipping already scheduled but not completed job", zap.String("jobName", job.Name), zap.String("jobID", job.Id), zap.String("selfJobList", joblist2string(list)), zap.Any("zapJobList", list))
			continue
		}

		// If the job is expired (job.EndTime < time.Now) a 'failed' job-status is sent to the server
		if time.Now().Unix() > job.EndTime {
			apglog.Debug("Expired job found: send 'failed' status", zap.Any("oldJob", job.Name))
			err = api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				apglog.Error("Unable to send 'failed' status to expired job", zap.String("oldJob", job.Name))
			}
			continue
		}

		// Check if the endTime of the task is proper set (after startTime & max 24h long)
		if job.StartTime > job.EndTime || (job.EndTime-job.StartTime) > maxJobDuration {
			apglog.Error("Invalid job details: Job potentially running too long", zap.String("job", job.Name))
			err = api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				apglog.Error("Unable to send 'failed' status of too long running job", zap.String("job", job.Name), zap.NamedError("statusError", err))
			}
			continue
		}

		// For now only single shot tasks are supported
		// todo: as we can only run one task at a time, figure out some "timeout" mechanism that terminates stuck jobs
		_, err := h.scheduler.Tag(job.Id).Every(1).Millisecond().LimitRunsTo(1).StartAt(time.Unix(job.StartTime, 0)).DoWithJobDetails(handlerFunc, params)
		if err != nil {
			apglog.Error("Error during scheduling job", zap.String("job", job.Name), zap.NamedError("schedulingError", err))
			err = api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				apglog.Error("Unable to send 'failed' status after errored job scheduling", zap.String("job", job.Name), zap.NamedError("statusError", err))
			}
		}
	}
	if len(skippedJobs) > 0 {
		apglog.Debug("Skipped already scheduled but not completed jobs", zap.Any("skippedJobList", skippedJobs))
	}

}

func NewJobHandler(app *apogee.App) (*JobHandler, error) {
	jh := &JobHandler{}
	jh.app = app

	// Set up the rest api backend
	backend, err := backend.NewRestAPIBackend(app)
	jh.backend = backend

	jh.scheduler = gocron.NewScheduler(time.UTC)
	// Force 1 concurrent job, and reschedule if not possible
	// As we use one-off tasks this does not re-schedule so we rely on our server returning the job on the next poll!
	jh.scheduler.SetMaxConcurrentJobs(1, gocron.RescheduleMode)
	jh.scheduler.StartAsync()

	return jh, err
}
