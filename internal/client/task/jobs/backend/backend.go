package backend

import (
	"github.com/LeoCommon/client/internal/client/task/jobs/schema"
	"github.com/LeoCommon/client/internal/client/task/scheduler"
)

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*schema.JobParameters) (scheduler.JobFunction, scheduler.ExclusiveResources)
}
