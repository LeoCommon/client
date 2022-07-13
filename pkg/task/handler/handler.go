package handler

// This defines a generic handler that manages jobs

import (
	"fmt"
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

	fmt.Printf("job list %v", newJobs)

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
			apglog.Debug("Skipping already scheduled but not completed job")
			continue
		}

		// For now only single shot tasks are supported
		h.scheduler.Tag(job.Id).Every(1).Millisecond().LimitRunsTo(1).StartAt(time.Unix(job.StartTime, 0)).DoWithJobDetails(handlerFunc, params)
	}

}

func NewJobHandler(app *apogee.App) (*JobHandler, error) {
	jh := &JobHandler{}

	jh.scheduler = gocron.NewScheduler(time.UTC)

	// Force 1 concurrent job, and reschedule if not possible (skips one-off jobs entirely!)
	jh.scheduler.SetMaxConcurrentJobs(1, gocron.RescheduleMode)
	jh.scheduler.StartAsync()

	jh.app = app

	backend, err := backend.NewRestAPIBackend(app)
	jh.backend = backend

	return jh, err
}
