package backend

import (
	"fmt"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/task/jobs"

	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
)

func (h *restAPIBackend) Initialize(app *apogee.App) {
	// Tell the server you are alive
	myStatus, _ := jobs.GetDefaultSensorStatus(app)
	err := api.PutSensorUpdate(myStatus)
	if err != nil {
		apglog.Error("unable to put sensor update on server: " + err.Error())
	}
}

type restAPIBackend struct {
}

// GetJobHandlerFromParameters implements Backend

func (h *restAPIBackend) GetJobHandlerFromParameters(jp *JobParameters) JobFunction {
	switch jp.Job.(type) {
	case api.FixedJob:
		return h.handleFixedJob
	default:
		apglog.Error("Unsupported job type passed to rest api backend", zap.Any("type", jp.Job))
		return nil
	}
}

// This is a dynamic task selection because we need to be able to run POST Hooks
func (b *restAPIBackend) handleFixedJob(param interface{}, gcJob gocron.Job) {
	jp := param.(*JobParameters)

	apiJob := jp.Job.(api.FixedJob)
	cmd := strings.ToLower(apiJob.Command)
	jobName := apiJob.Name

	var err error
	if strings.Contains("get_status, push_status, return_status, small_status", cmd) {
		err = jobs.PushStatus(jp.App)
	} else if strings.Contains("get_full_status, get_verbose_status, get_big_status", cmd) {
		err = jobs.ReportFullStatus(jobName, jp.App)
	} else if strings.Contains("iridium_sniffing, iridiumsniffing", cmd) {
		err = jobs.IridiumSniffing(apiJob, jp.App)
	} else if strings.Contains("get_logs", cmd) {
		serviceName := apiJob.Arguments["service"]
		if len(serviceName) == 0 {
			serviceName = "foobar.doesntexist"
		}
		err = jobs.GetLogs(jobName, jp.App, serviceName)
	} else {
		err = fmt.Errorf("unsupported job was sent to the client")
	}

	// This error handling is meh, please improve
	if err == nil {
		err = api.PutJobUpdate(jobName, "finished")
		if err != nil {
			apglog.Info("Put Job " + jobName + " into 'finished' did not work: " + err.Error())
		}
	} else {
		err = api.PutJobUpdate(jobName, "failed")
		if err != nil {
			apglog.Info("Put Job " + jobName + " into 'failed' did not work: " + err.Error())
		}
	}
}

func NewRestAPIBackend(app *apogee.App) (Backend, error) {
	b := &restAPIBackend{}

	return b, nil
}
