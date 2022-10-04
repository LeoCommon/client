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

		// Ignore the error of this function its not really an "error"
		list, _ := h.scheduler.FindJobsByTag(job.Id)
		if len(list) > 0 {
			apglog.Debug("Skipping already scheduled but not completed job", zap.Any("job", list))
			continue
		}

		if time.Now().Unix() > job.StartTime && time.Now().Unix() > job.EndTime {
			apglog.Debug("Expired job found: send 'failed' status", zap.Any("oldJob", job.Name))
			err = api.PutJobUpdate(job.Name, "failed")
			if err != nil {
				apglog.Error("Unable to send 'failed' status to expired job", zap.String("oldJob", job.Name))
			}
			continue
		}

		// For now only single shot tasks are supported
		// todo: as we can only run one task at a time, figure out some "timeout" mechanism that terminates stuck jobs
		h.scheduler.Tag(job.Id).Every(1).Millisecond().LimitRunsTo(1).StartAt(time.Unix(job.StartTime, 0)).DoWithJobDetails(handlerFunc, params)
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
