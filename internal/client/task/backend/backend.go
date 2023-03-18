package backend

import (
	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/task/scheduler"
)

type JobParameters struct {
	Job interface{}

	App *client.App
}

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*JobParameters) (scheduler.JobFunction, scheduler.ExclusiveResources)
}
