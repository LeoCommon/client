package backend

import (
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/schema"
	"disco.cs.uni-kl.de/apogee/internal/client/task/scheduler"
)

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*schema.JobParameters) (scheduler.JobFunction, scheduler.ExclusiveResources)
}
