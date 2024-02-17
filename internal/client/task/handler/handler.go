package handler

// This defines a generic handler that manages jobs

import (
	"go.uber.org/zap"
	"runtime"
	"strings"
	"sync"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/backend"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/schema"
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
func (h *TaskHandler) MarkFailed(job api.FixedJob, details string) {
	if len(details) < 1 {
		go h.app.Api.PutJobUpdate(job.Name, "failed")
	} else {
		details = strings.ReplaceAll(details, " ", "_")
		go h.app.Api.PutJobUpdate(job.Name, "failed("+details+")")
	}
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
		params := &schema.JobParameters{}
		params.Job = job
		params.App = h.app
		params.Config = h.app.Conf.Job().C()

		// fixme: as long as we use the task.name as identifier we need it to be set
		if len(job.Name) == 0 {
			h.MarkFailed(job, "no jobName")
			continue
		}

		// If the jobs endTime is already expired, mark it as failed
		endTime := job.EndTime
		if time.Now().After(endTime) {
			h.MarkFailed(job, "expired executionTime")
			continue
		}

		// If the job is already marked as running for our sensor, also skip it
		myName := h.app.Conf.SensorName()
		myStatus := job.States[myName]
		if (len(myStatus) != 0) && (myStatus != "pending") {
			log.Info("skipping to enqueue already running job", zap.String("job", job.Json()))
			continue
		}

		// If no job handler was found, mark as failed and continue
		handlerFunc, exclusiveResources := h.backend.GetJobHandlerFromParameters(params)
		if handlerFunc == nil {
			log.Error("no handler for job", zap.String("job", job.Json()))
			h.MarkFailed(job, "no handler")
			continue
		}

		// Create a new task object
		task := scheduler.
			NewTask(job.StartTime, job.EndTime, handlerFunc, params).
			WithID(job.Id).
			WithResource(exclusiveResources...)

		// Schedule it
		err := h.scheduler.Schedule(task)
		if err != nil {
			// Unstarted tasks are allowed to be updated, so dont error out on AlreadyExists
			// And if the identical task is already running, we also dont do anything
			if err == scheduler.ErrTaskAlreadyExists || err == scheduler.ErrTaskAlreadyRunning {
				continue
			}

			log.Error("could not schedule job", zap.Error(err))
			h.MarkFailed(job, "schedulingError:"+err.Error())
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
