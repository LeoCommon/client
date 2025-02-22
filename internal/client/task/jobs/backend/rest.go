package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/LeoCommon/client/internal/client/api"
	"github.com/LeoCommon/client/internal/client/task/jobs"
	"github.com/LeoCommon/client/internal/client/task/jobs/iridium"
	"github.com/LeoCommon/client/internal/client/task/jobs/network"
	"github.com/LeoCommon/client/internal/client/task/jobs/schema"
	"github.com/LeoCommon/client/internal/client/task/scheduler"
	"github.com/LeoCommon/client/pkg/log"
	"github.com/LeoCommon/client/pkg/system/services/net"

	"go.uber.org/zap"
)

type restAPIBackend struct {
	api *api.RestAPI
}

// GetJobHandlerFromParameters implements Backend
func (h *restAPIBackend) GetJobHandlerFromParameters(jp *schema.JobParameters) (scheduler.JobFunction, scheduler.ExclusiveResources) {
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
	jp := param.(*schema.JobParameters)

	apiJob := jp.Job.(api.FixedJob)
	cmd := strings.ToLower(apiJob.Command)
	jobName := apiJob.Name
	//jobId := apiJob.Id

	log.Info("Job starting", zap.String("name", jobName), zap.String("command", cmd), zap.Time("startTime", apiJob.StartTime), zap.Time("endTime", apiJob.EndTime))

	//runningErr := b.api.PutJobUpdate(jobId, "running")
	runningErr := b.api.PutJobUpdate(jobName, "running")
	if runningErr != nil {
		// if an error occurs here, do not continue. Something big is broken, this should always work.
		log.Error("push Job starting", zap.String("name", jobName), zap.NamedError("runningError", runningErr))
		return runningErr
	}

	var err error
	if strings.Contains("get_status", cmd) {
		err = jobs.PushStatus(jp)
	} else if strings.Contains("get_full_status", cmd) {
		err = jobs.ReportFullStatus(ctx, apiJob, jp)
	} else if strings.Contains("iridium_sniffing", cmd) {
		err = iridium.IridiumSniffing(ctx, apiJob, jp)
	} else if strings.Contains("get_logs", cmd) {
		err = jobs.GetLogs(ctx, apiJob, jp)
	} else if strings.Contains("reboot", cmd) {
		err = jobs.RebootSensor(apiJob, jp)
	} else if strings.Contains("reset", cmd) {
		// send a 'job finished' message, assuming everything worked. (You have no other chance to mark it as finished.)
		err = b.api.PutJobUpdate(jobName, "finished")
		if err != nil {
			log.Info("hasty push reset result 'finished'", zap.String("name", jobName), zap.NamedError("PutJobUpdate", err))
		}
		err = jobs.ForceReset()
	} else if strings.Contains("set_network_conn", cmd) {
		err = network.SetNetworkConnectivity(apiJob, jp)
	} else if strings.Contains("set_wifi_config", cmd) {
		err = network.SetConfig(apiJob, jp, net.WiFi)
	} else if strings.Contains("set_eth_config", cmd) {
		err = network.SetConfig(apiJob, jp, net.Ethernet)
	} else if strings.Contains("set_gsm_config", cmd) {
		err = network.SetConfig(apiJob, jp, net.GSM)
	} else if strings.Contains("get_sys_config", cmd) {
		err = jobs.GetConfig(ctx, apiJob, jp)
	} else if strings.Contains("set_sys_config", cmd) {
		err = jobs.SetConfig(apiJob, jp)
	} else {
		err = fmt.Errorf("unsupported job was sent to the client")
	}

	verb := "finished"
	if err != nil {
		errStr := strings.ReplaceAll(err.Error(), " ", "_")
		verb = "failed(" + errStr + ")"
	}

	//submitErr := b.api.PutJobUpdate(jobId, verb)
	submitErr := b.api.PutJobUpdate(jobName, verb)
	if submitErr != nil {
		// if an error occurs here, do not continue. Something big is broken, this should always work.
		log.Error("push Job result", zap.String("name", jobName), zap.String("status", verb), zap.NamedError("submitError", submitErr))
		return submitErr
	}
	log.Info("Job result change", zap.String("name", jobName), zap.NamedError("executionError", err), zap.String("finalState", verb))

	return nil
}

func NewRestAPIBackend(api *api.RestAPI) (Backend, error) {
	b := &restAPIBackend{
		api,
	}

	return b, nil
}
