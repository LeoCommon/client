package backend

import (
	"context"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"github.com/go-co-op/gocron"
)

type JobParameters struct {
	Job interface{}
	Ctx context.Context

	App *client.App
}

type JobType string

const (
	JobTypeDefault JobType = "Default"
	// Only permit a single singleton job at the given timeframe
	JobTypeSingleton JobType = "Singleton"
)

// Takes an additional context, so we can cancel
type JobFunction func(interface{}, gocron.Job)

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*JobParameters) (JobFunction, JobType)
}
