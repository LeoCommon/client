package schema

import (
	"github.com/LeoCommon/client/internal/client"
	"github.com/LeoCommon/client/internal/client/config"
)

type JobParameters struct {
	Job interface{}
	App *client.App

	// A copy of the jobConfig
	Config config.JobsConfig
}
