package jobs

import (
	"bytes"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"encoding/json"
	"errors"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
)

const maxSniffingTime = 3600

type SniffingConfig struct {
	CenterFrequency_khz float64
	Bandwidth_khz       float64
	Gain                int64
	Bb_gain             int64
	If_gain             int64
}

func parseFloat(inStr string, defVal float64, argument string) float64 {
	parsedValue, err := strconv.ParseFloat(inStr, 64)
	if err != nil {
		apglog.Info("Bad " + argument + " value: " + inStr)
		return defVal
	}
	return parsedValue
}

func parseInt(inStr string, defVal int64, argument string) int64 {
	parsedValue, err := strconv.ParseInt(inStr, 10, 64)
	if err != nil {
		apglog.Info("Bad " + argument + " value: " + inStr)
		return defVal
	}
	return parsedValue
}

func getJobStoragePath(jobName string, app *apogee.App) string {
	part1 := app.Config.Client.Jobs.TempCollectStorage
	if part1[len(part1)-1] != '/' {
		return part1 + "/" + jobName + "/"
	}
	return part1 + jobName + "/"
}

func getJobBufferPath(jobName string, app *apogee.App) string {
	part1 := app.Config.Client.Jobs.TempRecStorage
	if part1[len(part1)-1] != '/' {
		return part1 + "/" + jobName + "/"
	}
	return part1 + jobName + "/"
}

func RunCommandWithTimeout(timeout int, command string, args ...string) (stdout, stderr string, isKilled bool) {
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.Command(command, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	err := cmd.Start()
	if err != nil {
		apglog.Error("Error starting sniffing-process: " + err.Error())
		return "", "", false
	}
	//fmt.Printf("my pid: %d\n", os.Getpid())
	//fmt.Printf("child pid:%d\n", cmd.Process.Pid)
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()
	//cmd.Process.Release()
	after := time.After(time.Duration(timeout) * time.Millisecond)
	select {
	case <-after:
		//syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		if err != nil {
			apglog.Error("Error interrupting the sniffing-process: " + err.Error())
			return "", "", false
		}
		time.Sleep(200 * time.Millisecond)
		isKilled = true
	case <-done:
		isKilled = false
	}
	stdout = string(bytes.TrimSpace(stdoutBuf.Bytes())) // Remove \n
	stderr = string(bytes.TrimSpace(stderrBuf.Bytes())) // Remove \n
	return
}

func ParseArguments(args map[string]string) SniffingConfig {
	// assumed input format: key1=value1; key2:value2
	snCon := SniffingConfig{CenterFrequency_khz: 1621500, Bandwidth_khz: 5000, Gain: 14, If_gain: 40, Bb_gain: 20}
	// get the keys
	keys := make([]string, len(args))
	i := 0
	for k := range args {
		keys[i] = k
		i++
	}
	// go though the keys
	for i := 0; i < len(keys); i++ {
		tempKey := keys[i]
		tempValue := args[tempKey]
		tempKey = strings.ToLower(tempKey)
		if strings.Contains(tempKey, "centerfrequency_mhz") {
			snCon.CenterFrequency_khz = 1000.0 * parseFloat(tempValue, 1621.5, "centerfrequency_mhz")
		} else if strings.Contains(tempKey, "bandwidth_mhz") {
			snCon.Bandwidth_khz = 1000.0 * parseFloat(tempValue, 5.0, "bandwidth_mhz")
		} else if strings.Contains(tempKey, "bandwidth_khz") {
			snCon.Bandwidth_khz = parseFloat(tempValue, 5000, "bandwidth_khz")
		} else if strings.Contains(tempKey, "bb_gain") {
			snCon.Bb_gain = parseInt(tempValue, 14, "bb_gain")
		} else if strings.Contains(tempKey, "if_gain") {
			snCon.If_gain = parseInt(tempValue, 40, "if_gain")
		} else if strings.Contains(tempKey, "gain") {
			snCon.Gain = parseInt(tempValue, 20, "gain")
		} else {
			apglog.Info("Unknown iridium-sniffing argument: " + tempKey + ":" + tempValue)
		}
	}

	return snCon
}

func writeJobInfoFile(fileCollection []string, job api.FixedJob, app *apogee.App) ([]string, error) {
	var outErr error = nil
	jobFileName := job.Name + "_job.txt"
	jobFilePath := getJobStoragePath(job.Name, app) + jobFileName
	jobString, err := json.Marshal(job)
	if err != nil {
		apglog.Error("Error encoding the job-string: " + err.Error())
		outErr = err
	}
	_, err = files.WriteInFile(jobFilePath, string(jobString))
	if err != nil {
		apglog.Error("Error writing the job-file: " + err.Error())
		outErr = err
	} else {
		fileCollection = append(fileCollection, jobFilePath)
	}
	return fileCollection, outErr
}

func writeStatusFile(statusType string, fileCollection []string, job api.FixedJob, app *apogee.App) ([]string, error) {
	statusFileName := job.Name + "_" + statusType + ".txt"
	statusFilePath := getJobStoragePath(job.Name, app) + statusFileName
	status, _ := GetDefaultSensorStatus(app)
	statusString, err := json.Marshal(status)
	var outErr error = nil
	if err != nil {
		apglog.Error("Error encoding the " + statusType + ": " + err.Error())
		outErr = err
	}
	_, err = files.WriteInFile(statusFilePath, string(statusString))
	if err != nil {
		apglog.Error("Error writing the " + statusType + "-file: " + err.Error())
		outErr = err
	} else {
		fileCollection = append(fileCollection, statusFilePath)
	}
	return fileCollection, outErr
}

func writeHackrfConfigFile(job api.FixedJob, app *apogee.App) (string, error) {
	snCon := ParseArguments(job.Arguments)
	configString := "[osmosdr-source]\n" +
		"sample_rate=" + strconv.FormatInt(int64(snCon.Bandwidth_khz*1000), 10) +
		"\ncenter_freq=" + strconv.FormatInt(int64(snCon.CenterFrequency_khz*1000), 10) +
		"\nbandwidth=" + strconv.FormatInt(int64(snCon.Bandwidth_khz*1000), 10) +
		"\ngain=" + strconv.FormatInt(snCon.Gain, 10) +
		"\nif_gain=" + strconv.FormatInt(snCon.If_gain, 10) +
		"\nbb_gain=" + strconv.FormatInt(snCon.Bb_gain, 10) +
		"\n"
	configFileName := "hackrf.conf"
	configFilePath := getJobStoragePath(job.Name, app) + configFileName
	_, err := files.WriteInFile(configFilePath, configString)
	if err != nil {
		apglog.Error("Error writing the hackrf.conf-file: " + err.Error())
		return "", err
	}
	return configFilePath, nil
}

func writeErrorLogFile(fileCollection []string, job api.FixedJob, app *apogee.App) ([]string, error) {
	serviceName := "apogee-client.service"
	filePath := getJobBufferPath(job.Name, app) + "errorLog.txt"
	serviceLogs, err := cli.GetServiceLogs(serviceName)
	if err != nil {
		apglog.Error("Error reading serviceLogs: " + err.Error())
		serviceLogs = serviceLogs + err.Error()
	}
	_, err = files.WriteInFile(filePath, serviceLogs)
	if err != nil {
		apglog.Error("Error writing log file: " + err.Error())
		return fileCollection, err
	}
	fileCollection = append(fileCollection, filePath)
	return fileCollection, nil
}

func zipAndUpload(fileCollection []string, job api.FixedJob, app *apogee.App) error {
	// zip all files (job-file + start-/end-status + sniffing files)
	archiveName := "job_" + job.Name + "_sensor_" + app.SensorName() + ".zip"
	archivePath := app.Config.Client.Jobs.TempCollectStorage + archiveName
	_, err := files.WriteFilesInArchive(archivePath, fileCollection)
	if err != nil {
		apglog.Error("Could not zip iridium sniffing files", zap.Error(err))
		return err
	}
	// upload zip to server
	err = api.PostSensorData(job.Name, archiveName, archivePath)
	if err != nil {
		apglog.Error("Error uploading job-archive to server ", zap.Error(err))
	}
	// remove archive, that storage is not filled up
	err = os.Remove(archivePath)
	if err != nil {
		apglog.Error("Error removing job-archive", zap.Error(err))
	}
	return err
}

func cleanup(fileCollection []string, job api.FixedJob, app *apogee.App) error {
	var outErr error = nil
	// remove all raw files
	for i := 0; i < len(fileCollection); i++ {
		err := os.Remove(fileCollection[i])
		if err != nil {
			apglog.Error("Error deleting file: " + err.Error())
			outErr = err
		}
	}

	err := os.Remove(getJobStoragePath(job.Name, app))
	if err != nil {
		apglog.Error("Error deleting job-folder: " + err.Error())
		outErr = err
	}
	return outErr
}

func IridiumSniffing(job api.FixedJob, app *apogee.App) error {
	// get all starting information
	endTime := job.EndTime
	var sniffingFilePaths []string
	cumulativeErr := error(nil)
	jobFolder := getJobStoragePath(job.Name, app)

	// write job-info into file
	sniffingFilePaths, err := writeJobInfoFile(sniffingFilePaths, job, app)
	if err != nil {
		cumulativeErr = err
	}

	// write start-status into file
	sniffingFilePaths, err = writeStatusFile("startStatus", sniffingFilePaths, job, app)
	if err != nil {
		cumulativeErr = err
	}

	//write the config-files for sniffing
	configFilePath, err := writeHackrfConfigFile(job, app)
	if err != nil {
		cleanup(sniffingFilePaths, job, app)
		return err
	}
	sniffingFilePaths = append(sniffingFilePaths, configFilePath)

	// perform the sniffing
	timeRemaining := endTime - time.Now().Unix()
	for timeRemaining > 0 {
		// determine the sniffing duration
		executionDuration := timeRemaining
		if executionDuration > maxSniffingTime {
			executionDuration = maxSniffingTime
		}
		// store actual sniffing files in /tmp/job_files
		sniffingFileName := strconv.FormatInt(time.Now().Unix(), 10) + ".bits"
		tmpSniffingPath := getJobBufferPath(job.Name, app) + sniffingFileName
		_, err = files.WriteInFile(tmpSniffingPath, "")
		if err != nil {
			apglog.Error("Error creating sniffing-buffer-File: " + err.Error())
			cumulativeErr = err
		}
		// do the sniffing
		iridiumExtractorSh := "/etc/apogee/execute_gr_iridium.sh"
		apglog.Debug("Start sniffing iridium")
		_, stderr, _ := RunCommandWithTimeout(int(executionDuration*1000), "sh", iridiumExtractorSh, configFilePath, tmpSniffingPath)
		apglog.Debug("End sniffing iridium")
		if strings.Contains(stderr, "Using HackRF One") {
			apglog.Debug("Sniffing iridium seems to be successful")
		} else if strings.Contains(stderr, "Resource busy") {
			apglog.Error("Error during sniffing iridium: Resource busy")
			cumulativeErr = errors.New("error during sniffing iridium: resource busy")
			_, err = files.WriteInFile(tmpSniffingPath, stderr)
			if err != nil {
				apglog.Error("Error writing recording-busy-error in tmp-sniffing-File: " + err.Error())
			}
			// the only thing that currently helps to recover is a reboot, maybe in the future something like this could help: https://askubuntu.com/questions/645/how-do-you-reset-a-usb-device-from-the-command-line
			// for now upload everything you have and reboot
			sniffingFilePaths, _ = writeStatusFile("errorStatus", sniffingFilePaths, job, app)
			sniffingFilePaths, _ = writeErrorLogFile(sniffingFilePaths, job, app)
			_ = zipAndUpload(sniffingFilePaths, job, app)
			_ = cleanup(sniffingFilePaths, job, app)
			err := cli.RebootSystem()
			if err != nil {
				apglog.Error("Could not reboot with busy hackRF One: " + err.Error())
				return err
			}
		} else if strings.Contains(stderr, "No supported devices found") {
			apglog.Error("Error during sniffing iridium: No supported devices found")
			cumulativeErr = errors.New("error during sniffing iridium: no supported devices found")
			_, err = files.WriteInFile(tmpSniffingPath, stderr)
			if err != nil {
				apglog.Error("Error writing recording-no-devices-error in tmp-sniffing-File: " + err.Error())
			}
		} else {
			apglog.Error("Unknown output:" + stderr)
			cumulativeErr = errors.New("unknown state:" + stderr)
		}

		// copy them to the usb-stick
		bigSniffingPath := jobFolder + "/" + sniffingFileName
		err := files.MoveFile(tmpSniffingPath, bigSniffingPath)
		if err != nil {
			apglog.Info("Error moving the tmp-sniffing-file: " + err.Error())
			cumulativeErr = err
		} else {
			sniffingFilePaths = append(sniffingFilePaths, bigSniffingPath)
		}

		//figure out the remaining time
		timeRemaining = endTime - time.Now().Unix()
	}

	// write end-status into file
	sniffingFilePaths, err = writeStatusFile("endStatus", sniffingFilePaths, job, app)
	if err != nil {
		cumulativeErr = err
	}

	// zip all files (job-file + start-/end-status + sniffing files) and upload them
	err = zipAndUpload(sniffingFilePaths, job, app)
	if err != nil {
		cumulativeErr = err
	}

	// remove all files
	err = cleanup(sniffingFilePaths, job, app)
	if err != nil {
		cumulativeErr = err
	}

	// return job errors
	if cumulativeErr != nil {
		return cumulativeErr
	} else {
		return nil
	}
}
