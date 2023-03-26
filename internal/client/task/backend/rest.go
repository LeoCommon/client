package backend

import (
	"context"
	"fmt"
	"strings"

	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/iridium"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/network"
	"disco.cs.uni-kl.de/apogee/internal/client/task/scheduler"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"

	"go.uber.org/zap"
)

type restAPIBackend struct {
	api *api.RestAPI
}

// GetJobHandlerFromParameters implements Backend
func (h *restAPIBackend) GetJobHandlerFromParameters(jp *JobParameters) (scheduler.JobFunction, scheduler.ExclusiveResources) {
	if fj, ok := jp.Job.(api.FixedJob); ok {
		resources := scheduler.ExclusiveResources{}

		// fixme: this should ideally be set on the server, or stored in a static mapping somewhere
		// If the job contains the word iridium, we need the SDR in exclusive mode
		if strings.Contains(strings.ToLower(fj.Command), "iridium") {
			resources = append(resources, scheduler.SDRDevice1)
		}

		return h.handleFixedJob, resources
	}

	log.Error("unsupported job type passed to the rest api backend", zap.Any("type", jp.Job))
	return nil, scheduler.ExclusiveResources{}
}

// This is a dynamic task selection because we need to be able to run POST Hooks
func (b *restAPIBackend) handleFixedJob(ctx context.Context, param interface{}) error {
	jp := param.(*JobParameters)

	apiJob := jp.Job.(api.FixedJob)
	cmd := strings.ToLower(apiJob.Command)
	jobName := apiJob.Name

	runningErr := b.api.PutJobUpdate(jobName, "running")
	log.Info("Job starting", zap.String("name", jobName), zap.String("command", cmd), zap.Time("startTime", apiJob.StartTime), zap.Time("endTime", apiJob.EndTime))

	var err error
	if strings.Contains("get_status, push_status, return_status, small_status", cmd) {
		err = jobs.PushStatus(jp.App)
	} else if strings.Contains("get_full_status, get_verbose_status, get_big_status", cmd) {
		err = jobs.ReportFullStatus(ctx, jobName, jp.App)
	} else if strings.Contains("iridium_sniffing, iridiumsniffing", cmd) {
		err = iridium.IridiumSniffing(apiJob, ctx, jp.App)
	} else if strings.Contains("get_logs", cmd) {
		err = jobs.GetLogs(ctx, apiJob, jp.App)
	} else if strings.Contains("reboot", cmd) {
		err = jobs.RebootSensor(apiJob, jp.App)
	} else if strings.Contains("reset", cmd) {
		err = jobs.ForceReset()
	} else if strings.Contains("set_network_conn", cmd) {
		err = network.SetNetworkConnectivity(apiJob, jp.App)
	} else if strings.Contains("set_wifi_config", cmd) {
		err = network.SetConfig(apiJob, jp.App, net.WiFi)
	} else if strings.Contains("set_eth_config", cmd) {
		err = network.SetConfig(apiJob, jp.App, net.Ethernet)
	} else if strings.Contains("set_gsm_config", cmd) {
		err = network.SetConfig(apiJob, jp.App, net.GSM)
	} else {
		err = fmt.Errorf("unsupported job was sent to the client")
	}

	// todo: this error handling is meh, we should transmit more details here
	verb := "finished"
	if err != nil {
		verb = "failed"
	}

	submitErr := b.api.PutJobUpdate(jobName, verb)
	log.Info("Job result change", zap.String("name", jobName), zap.NamedError("setRunningError", runningErr), zap.NamedError("executionError", err), zap.String("finalState", verb), zap.NamedError("submitError", submitErr))

	// Return errors
	if runningErr == nil {
		return submitErr
	}
	return runningErr
}

func NewRestAPIBackend(api *api.RestAPI) (Backend, error) {
	b := &restAPIBackend{api}

	return b, nil
}
