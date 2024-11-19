package iridium

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/constants"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/schema"
	"disco.cs.uni-kl.de/apogee/pkg/file"
	"disco.cs.uni-kl.de/apogee/pkg/misc"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/streamhelpers"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/log"
)

func (j *SniffingJob) ParseJobArguments() {
	j.config = SniffingConfig{
		CenterfrequencyKhz: 1621500,
		BandwidthKhz:       5000,
		Gain:               14,
		IfGain:             40,
		BbGain:             20,
	}

	// Get all arguments
	for key, value := range j.job.Arguments {
		// Convert key to lowercase and trim
		key = strings.TrimSpace(strings.ToLower(key))

		switch key {
		case "centerfrequency_mhz":
			j.config.CenterfrequencyKhz = 1000.0 * misc.ParseFloat(value, j.config.CenterfrequencyKhz/1000.0, key)
		case "bandwidth_mhz":
			j.config.BandwidthKhz = 1000.0 * misc.ParseFloat(value, j.config.BandwidthKhz/1000.0, key)
		case "bandwidth_khz":
			j.config.BandwidthKhz = misc.ParseFloat(value, j.config.BandwidthKhz, key)
		case "bb_gain":
			j.config.BbGain = misc.ParseInt(value, j.config.BbGain, key)
		case "if_gain":
			j.config.IfGain = misc.ParseInt(value, j.config.IfGain, key)
		case "gain":
			j.config.Gain = misc.ParseInt(value, j.config.Gain, key)
		default:
			log.Warn("unknown iridium-sniffing argument", zap.String("key", key), zap.String("value", value))
		}
	}
}

func (j *SniffingJob) getJobStoragePath() string {
	return filepath.Join(j.app.Conf.JobStoragePath(), j.job.Name)
}

func (j *SniffingJob) getJobFileName(suffix string) string {
	return j.job.Name + suffix
}

func (j *SniffingJob) addOutputFile(path string) {
	j.outputFiles = append(j.outputFiles, path)
}

func (j *SniffingJob) writeJobInfoFile() error {
	jobString, err := json.Marshal(j.job)
	if err != nil {
		log.Error("error encoding the job-string: " + err.Error())
		return err
	}

	jobFilePath := filepath.Join(j.getJobStoragePath(), j.getJobFileName("_job.txt"))
	err = file.WriteTo(jobFilePath, string(jobString))
	if err != nil {
		log.Error("Error writing the job-file", zap.String("file", jobFilePath))
		return err
	}

	// Add output file to the list
	j.addOutputFile(jobFilePath)

	return nil
}

func (j *SniffingJob) getStatusFilePath(statusType StatusType) string {
	return filepath.Join(
		j.getJobStoragePath(),
		fmt.Sprintf("%s_%s.txt", j.job.Name, string(statusType)),
	)
}

func (j *SniffingJob) writeStatusFile(jobStatus StatusType) error {
	sensorStatus, err := jobs.GetDefaultSensorStatus(j.app)
	if err != nil {
		log.Error("errors encountered when fetching default sensor status")
		return err
	}

	status, err := json.Marshal(sensorStatus)
	if err != nil {
		log.Error("marshalling failed for status")
		return err
	}

	statusFilePath := j.getStatusFilePath(jobStatus)
	err = file.WriteTo(statusFilePath, string(status))
	if err != nil {
		log.Error("error writing the jobStatusFile", zap.String("file", statusFilePath))
		return err
	}

	// Add the output file
	j.addOutputFile(statusFilePath)

	return nil
}

// This function writes the hackrf sdr config
// #todo this could use some stricter templating
func (j *SniffingJob) writeHackrfConfigFile() error {
	// Prepare the hackrf config string
	configContent := fmt.Sprintf(HackrfConfigTemplate,
		int64(j.config.BandwidthKhz*1000),
		int64(j.config.CenterfrequencyKhz*1000),
		int64(j.config.BandwidthKhz*1000),
		j.config.Gain,
		j.config.IfGain,
		j.config.BbGain,
	)

	// Assign config path for iridium-extractor
	j.configFilePath = filepath.Join(j.getJobStoragePath(), "hackrf.conf")

	err := file.WriteTo(j.configFilePath, configContent)
	if err != nil {
		log.Error("Error writing the hackrf.conf file", zap.String("file", j.configFilePath))
		return err
	}

	// Add the output file
	j.addOutputFile(j.configFilePath)

	return nil
}

func (j *SniffingJob) writeServiceLogFile() error {
	// Grab the service logs for apogee
	serviceLogs, err := cli.GetServiceLogs(constants.ClientServiceName)
	if err != nil {
		return err
	}

	serviceLogPath := filepath.Join(j.getJobStoragePath(), "serviceLog.txt")
	err = file.WriteTo(serviceLogPath, serviceLogs)
	if err != nil {
		log.Error("Error writing service log file", zap.String("file", serviceLogPath))
		return err
	}

	// Add the output file
	j.addOutputFile(serviceLogPath)

	return nil
}

func (j *SniffingJob) getArchiveName() string {
	return fmt.Sprintf("job_%s_sensor_%s.zip", j.job.Name, j.app.Conf.SensorName())
}

func (j *SniffingJob) zipAndUpload(ctx context.Context) error {
	// zip all files (job-file + start-/end-status + sniffing files)
	archiveName := j.getArchiveName()
	archivePath := filepath.Join(j.getJobStoragePath(), archiveName)

	err := file.CreateArchive(archivePath, j.outputFiles, j.getJobStoragePath())
	if err != nil {
		log.Error("Could not zip iridium sniffing files")
		return err
	}

	// remove archive, that storage is not filled up
	defer func(name string) {
		_ = os.Remove(name)
	}(archivePath)

	// upload zip to server
	err = j.app.Api.PostSensorData(ctx, j.job.Id, archivePath)
	if err != nil {
		log.Error("Error uploading job-archive to server", zap.Error(err))
	}

	return err
}

func (j *SniffingJob) cleanup() error {
	// Delete the entire job storage folder
	err := os.RemoveAll(j.getJobStoragePath())
	if err != nil {
		log.Error("Error deleting job-folder")
	}

	// Clear output file list
	j.outputFiles = nil

	return err
}

func monitorIridiumSniffingStartup(scanner *bufio.Scanner) error {
	result := make(chan error)
	go func() {
		for scanner.Scan() {
			line := strings.ToLower(scanner.Text())
			log.Debug("got output from startup check", zap.String("line", line))
			for _, check := range StartupCheckStrings {
				if !strings.Contains(line, check.Str) {
					continue
				}

				// The string was found, lets do what we need to do
				result <- check.Err
				return
			}
		}

		// If the process terminated early we will forward this, fill this with the real error later
		result <- streamhelpers.NewTerminatedEarlyError(nil)
	}()

	select {
	// Forward the result of our check function
	case err := <-result:
		return err
	// Same for the timeout
	case <-time.After(StartupCheckTimeout):
		return misc.NewTimedOutError("startup check timed", StartupCheckTimeout)
	}
}

func IridiumSniffing(ctx context.Context, job api.FixedJob, jp *schema.JobParameters) error {
	if jp.Config.Iridium.Disabled {
		return jobs.ErrJobDisabled
	}

	// Create sniffing data type
	j := SniffingJob{
		job: job,
		app: jp.App,
	}

	// Parse the job arguments and populate the required fields
	j.ParseJobArguments()

	// Clean up after we are done
	defer func(j *SniffingJob) {
		_ = j.cleanup()
	}(&j)

	// Add job info into the archive
	err := j.writeJobInfoFile()
	if err != nil {
		return err
	}

	// Add start status into the archive
	err = j.writeStatusFile(StatusTypeStart)
	if err != nil {
		log.Error("could not add start status to the job output", zap.Error(err))
	}

	// Create and add the config file to the archive
	err = j.writeHackrfConfigFile()
	if err != nil {
		return err
	}

	// Open the sniffing output in write-only mode
	captureOutputPath := filepath.Join(j.getJobStoragePath(), "output.bits")
	sniffingOutput := streamhelpers.NewCaptureFile(captureOutputPath).WithFlags(os.O_WRONLY | os.O_CREATE | os.O_TRUNC)
	j.addOutputFile(captureOutputPath)

	// Open the stderr log file in write-only mode
	errorOutputPath := filepath.Join(j.getJobStoragePath(), "output.stderr")
	logOutput := streamhelpers.NewCaptureFile(errorOutputPath).WithFlags(os.O_WRONLY | os.O_CREATE | os.O_TRUNC)
	j.addOutputFile(errorOutputPath)

	// Create a child context so we can also cancel at-will
	processCTX, cancel := context.WithCancel(ctx)

	// Construct the BufferedSTDReader
	cmdReader := streamhelpers.NewSTDReader(
		exec.Command("iridium-extractor", j.configFilePath),
		// Add the context
		processCTX,
	)

	// Add the file destinations
	err = cmdReader.WithFiles(streamhelpers.CaptureFiles{
		StdOUT: sniffingOutput,
		StdERR: logOutput,
	})

	if err != nil {
		cancel()
		return err
	}

	// gr-iridium handles SIGINT and completes with "done"
	cmdReader.SetTerminationSignal(syscall.SIGINT)

	// Create the pipe we are using for interactive reading
	stdErrReader, stdErrPipeWriter := io.Pipe()
	cmdReader.AttachStream(streamhelpers.StderrOut, stdErrPipeWriter, 0)

	// Start the process (async)
	cmdReader.Start()

	// Block and check for common error symptoms in the stream
	err = monitorIridiumSniffingStartup(bufio.NewScanner(stdErrReader))

	// Detach and close from the pipe
	cmdReader.DetachStream(stdErrPipeWriter)
	stdErrReader.Close()

	// If there was some sort of error, abort now
	if err != nil {
		log.Warn("startup error encountered, cancelling and forwarding error", zap.Error(err))
		// cancel the cmd context, so the process terminates (if it did not already)
		cancel()

		// One exception, if the startup returned EarlyExit, we need to get the real reason:
		if errors.Is(err, &streamhelpers.TerminatedEarlyError{}) {
			err = streamhelpers.NewTerminatedEarlyError(<-cmdReader.Wait())
		}

		// return the startup error
		return err
	}

	// Everything looks fine so far, wait for the sniffing job to terminate
	log.Info("startup successfull, sniffing now", zap.Error(err))
	defer cancel()

	// Wait for the result
	errFin := <-cmdReader.Wait()
	if errFin != nil {
		log.Error("sniffing job did not terminate correctly", zap.Error(errFin))
		//return err
	}

	// If Context was canceled do upload partial result (it helps to reveal problems)
	//if ctxErr := ctx.Err(); ctxErr == context.Canceled {
	//	return ctxErr
	//}

	// Add the end status file to the archive
	errStat := j.writeStatusFile(StatusTypeStop)
	if errStat != nil {
		log.Error("could not add end status to the job output", zap.Error(errStat))
	}

	// Add the service log file to the archive
	errLog := j.writeServiceLogFile()
	if errLog != nil {
		log.Error("could not add service log to the job output", zap.Error(errLog))
	}

	// zip all files (job-file + start-/end-status + sniffing files) and upload them
	// todo prepare some handler to cancel uploads
	errUp := j.zipAndUpload(context.TODO())

	if errFin != nil {
		return errFin
	}
	if errStat != nil {
		return errStat
	}
	if errLog != nil {
		return errLog
	}
	if errUp != nil {
		return errUp
	}

	return nil
}
