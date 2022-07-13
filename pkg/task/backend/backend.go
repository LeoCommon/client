package backend

import (
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"github.com/go-co-op/gocron"
)

type JobParameters struct {
	Job interface{}

	App *apogee.App
}

type JobFunction func(interface{}, gocron.Job)

type Backend interface {
	// Sets up required run time parameters
	GetJobHandlerFromParameters(*JobParameters) JobFunction
}
