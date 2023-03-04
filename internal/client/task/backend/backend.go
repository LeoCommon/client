package backend

import (
	"disco.cs.uni-kl.de/apogee/internal/client"
	"github.com/go-co-op/gocron"
)

type JobParameters struct {
	Job interface{}

	App *client.App
}

type JobFunction func(interface{}, gocron.Job)

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*JobParameters) JobFunction
}
