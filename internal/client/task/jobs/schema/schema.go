package schema

import (
	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
)

type JobParameters struct {
	Job interface{}
	App *client.App

	// A copy of the jobConfig
	Config config.JobsConfig
}
