package backend

import (
	"fmt"
	"strings"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
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

	runningErr := api.PutJobUpdate(jobName, "running")
	apglog.Info("Job starting", zap.String("name", jobName), zap.String("command", cmd), zap.Int64("startTime", apiJob.StartTime), zap.Int64("endTime", apiJob.EndTime))

	var err error
	if strings.Contains("get_status, push_status, return_status, small_status", cmd) {
		err = jobs.PushStatus(jp.App)
	} else if strings.Contains("get_full_status, get_verbose_status, get_big_status", cmd) {
		err = jobs.ReportFullStatus(jobName, jp.App)
	} else if strings.Contains("iridium_sniffing, iridiumsniffing", cmd) {
		err = jobs.IridiumSniffing(apiJob, jp.App)
	} else if strings.Contains("get_logs", cmd) {
		err = jobs.GetLogs(apiJob, jp.App)
	} else if strings.Contains("reboot", cmd) {
		err = jobs.RebootSensor(apiJob, jp.App)
	} else if strings.Contains("set_network_conn", cmd) {
		err = jobs.SetNetworkConnectivity(apiJob, jp.App)
	} else if strings.Contains("set_wifi_config", cmd) {
		err = jobs.SetNetworkConfig(apiJob, jp.App, net.WiFi)
	} else if strings.Contains("set_eth_config", cmd) {
		err = jobs.SetNetworkConfig(apiJob, jp.App, net.Ethernet)
	} else {
		err = fmt.Errorf("unsupported job was sent to the client")
	}

	// todo: this error handling is meh, we should transmit more details here
	verb := "finished"
	if err != nil {
		verb = "failed"
	}

	submitErr := api.PutJobUpdate(jobName, verb)
	apglog.Info("Job result change", zap.String("name", jobName), zap.NamedError("setRunningError", runningErr), zap.NamedError("executionError", err), zap.String("finalState", verb), zap.NamedError("submitError", submitErr))
}

func NewRestAPIBackend(app *apogee.App) (Backend, error) {
	b := &restAPIBackend{}

	return b, nil
}
