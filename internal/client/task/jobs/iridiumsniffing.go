package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/pkg/constants"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/misc"
	"disco.cs.uni-kl.de/apogee/pkg/system/streamhelpers"

	"go.uber.org/zap"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
)

type SniffingConfig struct {
	CenterFrequency_khz float64
	Bandwidth_khz       float64
	Gain                int64
	Bb_gain             int64
	If_gain             int64
}

type IridiumSniffingJob struct {
	app *client.App
	job api.FixedJob

	config         SniffingConfig
	configFilePath string

	// output file list
	outputFiles []string
}

func (j *IridiumSniffingJob) ParseJobArguments() {
	// assumed input format: key1=value1; key2:value2
	j.config = SniffingConfig{
		CenterFrequency_khz: 1621500,
		Bandwidth_khz:       5000,
		Gain:                14,
		If_gain:             40,
		Bb_gain:             20,
	}

	// Get all arguments
	for key, value := range j.job.Arguments {
		// Convert key to lowercase and trim
		key = strings.TrimSpace(strings.ToLower(key))

		switch key {
		case "centerfrequency_mhz":
			j.config.CenterFrequency_khz = 1000.0 * misc.ParseFloat(value, 1621.5, key)
		case "bandwidth_mhz":
			j.config.Bandwidth_khz = 1000.0 * misc.ParseFloat(value, 5.0, key)
		case "bandwidth_khz":
			j.config.Bandwidth_khz = misc.ParseFloat(value, 5000, key)
		case "bb_gain":
			j.config.Bb_gain = misc.ParseInt(value, 14, key)
		case "if_gain":
			j.config.If_gain = misc.ParseInt(value, 40, key)
		case "gain":
			j.config.Gain = misc.ParseInt(value, 20, key)
		default:
			log.Warn("unknown iridium-sniffing argument", zap.String("key", key), zap.String("value", value))
		}
	}
}

func (j *IridiumSniffingJob) getJobStoragePath() string {
	return filepath.Join(j.app.Config.Client.Jobs.StoragePath, j.job.Name)
}

func (j *IridiumSniffingJob) getJobFileName(suffix string) string {
	return j.job.Name + suffix
}

func (j *IridiumSniffingJob) addOutputFile(path string) {
	j.outputFiles = append(j.outputFiles, path)
}

func (j *IridiumSniffingJob) writeJobInfoFile() error {
	jobString, err := json.Marshal(j.job)
	if err != nil {
		log.Error("error encoding the job-string: " + err.Error())
		return err
	}

	jobFilePath := filepath.Join(j.getJobStoragePath(), j.getJobFileName("_job.txt"))
	err = files.WriteInFile(jobFilePath, string(jobString))
	if err != nil {
		log.Error("Error writing the job-file", zap.String("file", jobFilePath))
		return err
	}

	// Add output file to the list
	j.addOutputFile(jobFilePath)

	return nil
}

type StatusType string

const (
	StatusTypeStart StatusType = "startStatus"
	StatusTypeStop  StatusType = "endStatus"
)

func (j *IridiumSniffingJob) getStatusFilePath(statusType StatusType) string {
	return filepath.Join(
		j.getJobStoragePath(),
		fmt.Sprintf("%s_%s.txt", j.job.Name, string(statusType)),
	)
}

func (j *IridiumSniffingJob) writeStatusFile(jobStatus StatusType) error {
	sensorStatus, err := GetDefaultSensorStatus(j.app)
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
	err = files.WriteInFile(statusFilePath, string(status))
	if err != nil {
		log.Error("error writing the jobStatusFile", zap.String("file", statusFilePath))
		return err
	}

	// Add the output file
	j.addOutputFile(statusFilePath)

	return nil
}

const (
	HACKRF_CONFIG_TEMPLATE = `[osmosdr-source]
sample_rate=%d
center_freq=%d
bandwidth=%d
gain=%d
if_gain=%d
bb_gain=%d
`
)

// This function writes the hackrf sdr config
// #todo this could use some stricter templating
func (j *IridiumSniffingJob) writeHackrfConfigFile() error {
	// Prepare the hackrf config string
	configContent := fmt.Sprintf(HACKRF_CONFIG_TEMPLATE,
		int64(j.config.Bandwidth_khz*1000),
		int64(j.config.CenterFrequency_khz*1000),
		int64(j.config.Bandwidth_khz*1000),
		j.config.Gain,
		j.config.If_gain,
		j.config.Bb_gain,
	)

	// Assign config path for iridium-extractor
	j.configFilePath = filepath.Join(j.getJobStoragePath(), "hackrf.conf")

	err := files.WriteInFile(j.configFilePath, configContent)
	if err != nil {
		log.Error("Error writing the hackrf.conf file", zap.String("file", j.configFilePath))
		return err
	}

	// Add the output file
	j.addOutputFile(j.configFilePath)

	return nil
}

func (j *IridiumSniffingJob) writeServiceLogFile() error {
	// Grab the service logs for apogee
	serviceLogs, err := cli.GetServiceLogs(constants.APOGEE_SERVICE_NAME)
	if err != nil {
		return err
	}

	serviceLogPath := filepath.Join(j.getJobStoragePath(), "serviceLog.txt")
	err = files.WriteInFile(serviceLogPath, serviceLogs)
	if err != nil {
		log.Error("Error writing service log file", zap.String("file", serviceLogPath))
		return err
	}

	// Add the output file
	j.addOutputFile(serviceLogPath)

	return nil
}

func (j *IridiumSniffingJob) getArchiveName() string {
	return fmt.Sprintf("job_%s_sensor_%s.zip", j.job.Name, j.app.SensorName())
}

func (j *IridiumSniffingJob) zipAndUpload() error {
	// zip all files (job-file + start-/end-status + sniffing files)
	archiveName := j.getArchiveName()
	archivePath := filepath.Join(j.getJobStoragePath(), archiveName)

	err := files.WriteFilesInArchive(archivePath, j.outputFiles, j.getJobStoragePath())
	if err != nil {
		log.Error("Could not zip iridium sniffing files")
		return err
	}

	// remove archive, that storage is not filled up
	defer os.Remove(archivePath)

	// upload zip to server
	err = api.PostSensorData(j.job.Name, archiveName, archivePath)
	if err != nil {
		log.Error("Error uploading job-archive to server")
	}

	return err
}

func (j *IridiumSniffingJob) cleanup() error {
	// Delete the entire job storage folder
	err := os.RemoveAll(j.getJobStoragePath())
	if err != nil {
		log.Error("Error deleting job-folder")
	}

	// Clear output file list
	j.outputFiles = nil

	return err
}

func IridiumSniffing(job api.FixedJob, app *client.App) error {

	// Create sniffing data type
	j := IridiumSniffingJob{
		job: job,
		app: app,
	}

	// Parse the job arguments and populate the required fields
	j.ParseJobArguments()

	// Clean up after we are done
	defer j.cleanup()

	// Add job info into the archive
	err := j.writeJobInfoFile()
	if err != nil {
		return err
	}

	// Add start status into the archive
	err = j.writeStatusFile(StatusTypeStart)
	if err != nil {
		return err
	}

	// Create and add the config file to the archive
	err = j.writeHackrfConfigFile()
	if err != nil {
		return err
	}

	// Start doing the sniffy :)
	log.Debug("Start sniffing iridium")

	// Open the sniffing output in write-only mode
	captureOutputPath := filepath.Join(j.getJobStoragePath(), "output.bits")
	sniffingOutput := streamhelpers.NewCaptureFile(captureOutputPath).WithFlags(os.O_WRONLY | os.O_CREATE | os.O_TRUNC)
	j.addOutputFile(captureOutputPath)

	// Open the stderr log file in read-write mode
	errorOutputPath := filepath.Join(j.getJobStoragePath(), "output.stderr")
	logOutput := streamhelpers.NewCaptureFile(errorOutputPath)
	j.addOutputFile(errorOutputPath)

	// Create the context so the sniffing stops at the right time
	endTime := time.Unix(job.EndTime, 0)
	ctx, cancel := context.WithTimeout(context.Background(), endTime.Sub(time.Now().UTC()))
	defer cancel()

	// Construct the BufferedSTDReader
	cmdReader := streamhelpers.NewSTDReader(
		// Build the iridium sniffing command here
		exec.Command("iridium-extractor", "-D", "4", j.configFilePath), ctx).
		WithFiles(streamhelpers.CaptureFiles{
			StdOUT: sniffingOutput,
			StdERR: logOutput,
		}).
		SetTerminationSignal(syscall.SIGINT)

	var wg sync.WaitGroup

	// Run the process
	wg.Add(1)
	go func() {
		err := cmdReader.Capture()
		if err != nil {
			log.Error("error returned from capture", zap.Error(err))
		}
	}()

	// Check the output of the stream for the first 30 seconds to see if there were any faults

	wg.Wait()

	// #fixme todo post-process the error log to search for common faults in case we terminated early
	if err != nil {
		log.Fatal("STUB, implement me pls")
		return err
	}

	// Add the end status file to the archive
	err = j.writeStatusFile(StatusTypeStop)
	if err != nil {
		return err
	}

	// Add the service log file to the archive
	err = j.writeServiceLogFile()
	if err != nil {
		return err
	}

	// zip all files (job-file + start-/end-status + sniffing files) and upload them
	err = j.zipAndUpload()
	if err != nil {
		return err
	}

	// remove all files
	err = j.cleanup()
	if err != nil {
		return err
	}

	return nil
}
