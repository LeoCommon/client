package jobHandler

import (
	"bytes"
	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
	"disco.cs.uni-kl.de/apogee/pkg/system/files"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxSniffingTime = 3600

type SniffingConfig struct {
	CenterFrequency_khz float64
	Bandwidth_khz       float64
}

func parseFloat(inStr string, defVal float64, argument string) float64 {
	parsedValue, err := strconv.ParseFloat(inStr, 64)
	if err != nil {
		apglog.Info("Bad " + argument + " value: " + inStr)
		return defVal
	}
	return parsedValue
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
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err != nil {
			apglog.Error("Error killing the sniffing-process: " + err.Error())
			return "", "", false
		}
		time.Sleep(20 * time.Millisecond)
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
	snCon := SniffingConfig{}
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
		//check for other writing possibilities
		tempKey = strings.Replace(tempKey, "cf", "centerfrequency", -1)
		tempKey = strings.Replace(tempKey, "bw", "bandwidth", -1)
		//match khz and mhz
		if strings.Contains(tempKey, "centerfrequency_mhz") {
			snCon.CenterFrequency_khz = 1000.0 * parseFloat(tempValue, 1621.5, "centerfrequency_mhz")
		} else if strings.Contains(tempKey, "centerfrequency_khz") {
			snCon.CenterFrequency_khz = parseFloat(tempValue, 1621500, "centerfrequency_khz")
		} else if strings.Contains(tempKey, "centerfrequency") {
			// if no value is given, assume it is mhz
			snCon.CenterFrequency_khz = 1000.0 * parseFloat(tempValue, 1621.5, "centerfrequency")
		} else if strings.Contains(tempKey, "bandwidth_mhz") {
			snCon.Bandwidth_khz = 1000.0 * parseFloat(tempValue, 5.0, "bandwidth_mhz")
		} else if strings.Contains(tempKey, "bandwidth_khz") {
			snCon.Bandwidth_khz = parseFloat(tempValue, 5000, "bandwidth_khz")
		} else if strings.Contains(tempKey, "bandwidth") {
			// if no value is given, assume it is mhz
			snCon.Bandwidth_khz = 1000.0 * parseFloat(tempValue, 5.0, "bandwidth_mhz")
		} else {
			apglog.Info("Unknown iridium-sniffing argument: " + tempKey + ":" + tempValue)
		}
	}

	return snCon
}

func IridiumSniffing(job api.FixedJob, sensorName string) error {
	// get all starting information
	jobName := job.Name
	startTime := job.StartTime
	endTime := job.EndTime
	snCon := ParseArguments(job.Arguments)
	var sniffingFilePaths []string
	cumulativeErr := error(nil)
	jobFolder := bigStorage + "/" + jobName

	// write job-info into file
	jobFileName := jobName + "_job.txt"
	jobFilePath := jobFolder + "/" + jobFileName
	jobString, err := json.Marshal(job)
	if err != nil {
		apglog.Info("Error encoding the job-string: " + err.Error())
		cumulativeErr = err
	}
	_, err = files.WriteInFile(jobFilePath, string(jobString))
	if err != nil {
		apglog.Info("Error writing the job-file: " + err.Error())
		cumulativeErr = err
	} else {
		sniffingFilePaths = append(sniffingFilePaths, jobFilePath)
	}

	// write start-status into file
	startStatusFileName := jobName + "_startStatus.txt"
	startStatusFilePath := jobFolder + "/" + startStatusFileName
	startStatus, _ := GetDefaultSensorStatus()
	startStatusString, err := json.Marshal(startStatus)
	if err != nil {
		apglog.Info("Error encoding the start-status: " + err.Error())
		cumulativeErr = err
	}
	_, err = files.WriteInFile(startStatusFilePath, string(startStatusString))
	if err != nil {
		apglog.Info("Error writing the start-status-file: " + err.Error())
		cumulativeErr = err
	} else {
		sniffingFilePaths = append(sniffingFilePaths, startStatusFilePath)
	}

	//write the config-files for sniffing
	configString := "[osmosdr-source]\n" +
		"sample_rate=" + strconv.FormatInt(int64(snCon.Bandwidth_khz*1000), 10) +
		"\ncenter_freq=" + strconv.FormatInt(int64(snCon.CenterFrequency_khz*1000), 10) +
		"\nbandwidth=" + strconv.FormatInt(int64(snCon.Bandwidth_khz*1000), 10) +
		"\ngain=14" + "\nif_gain=40" + "\nbb_gain=20\n"
	configFileName := "hackrf.conf"
	configFilePath := jobFolder + "/" + configFileName
	_, err = files.WriteInFile(configFilePath, configString)
	if err != nil {
		apglog.Error("Error writing the hackrf.conf-file: " + err.Error())
		return err
	} else {
		sniffingFilePaths = append(sniffingFilePaths, configFilePath)
	}

	//check if sniffing is divided in more sniffing-parts (max 1h always)
	execTime := endTime - startTime
	execNumbers := 1
	if execTime > maxSniffingTime {
		execNumbers = int(math.Ceil(float64(execTime) / maxSniffingTime))
		execTime = int64(math.Ceil(float64(execTime) / float64(execNumbers)))
	}

	// perform the sniffing
	for i := 0; i < execNumbers; i++ {
		// store actual sniffing files in /tmp/job_files
		timeString := strconv.FormatInt(time.Now().Unix(), 10)
		sniffingFileName := jobName + "_" + timeString + ".bits"
		tmpSniffingPath := tmpStorage + "/" + sniffingFileName
		_, err = files.WriteInFile(tmpSniffingPath, "")
		if err != nil {
			apglog.Info("Error creating tmp-sniffing-File: " + err.Error())
			cumulativeErr = err
		}
		// do the sniffing
		iridiumExtractorSh := "/etc/apogee/execute_gr_iridium.sh"
		apglog.Debug("Start sniffing iridium")
		_, stderr, _ := RunCommandWithTimeout(int(execTime*1000), "sh", iridiumExtractorSh, configFilePath, tmpSniffingPath)
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

	}

	// write end-status into file
	endStatusFileName := jobName + "_endStatus.txt"
	endStatusFilePath := jobFolder + "/" + endStatusFileName
	endStatus, _ := GetDefaultSensorStatus()
	endStatusString, err := json.Marshal(endStatus)
	if err != nil {
		apglog.Info("Error encoding the end-status: " + err.Error())
	}
	_, err = files.WriteInFile(endStatusFilePath, string(endStatusString))
	if err != nil {
		apglog.Info("Error writing the end-status-file: " + err.Error())
		cumulativeErr = err
	} else {
		sniffingFilePaths = append(sniffingFilePaths, endStatusFilePath)
	}

	// zip all files (job-file + start-/end-status + sniffing files)
	archiveName := "job_" + jobName + "_sensor_" + sensorName + ".zip"
	archivePath := bigStorage + "/" + archiveName
	_, err = files.WriteFilesInArchive(archivePath, sniffingFilePaths)

	// remove all raw files
	for i := 0; i < len(sniffingFilePaths); i++ {
		err = os.Remove(sniffingFilePaths[i])
		if err != nil {
			apglog.Info("Error deleting file: " + err.Error())
			cumulativeErr = err
		}
	}
	err = os.Remove(jobFolder)
	if err != nil {
		apglog.Info("Error deleting job-folder: " + err.Error())
		cumulativeErr = err
	}

	// upload zip to server
	err = api.PostSensorData(jobName, archiveName, archivePath)
	if err != nil {
		apglog.Info("Error uploading job-archive to server: " + err.Error())
		cumulativeErr = err
	} else {
		err := os.Remove(archivePath)
		if err != nil {
			apglog.Info("Error removing job-archive: " + err.Error())
			cumulativeErr = err
		}
	}

	// return job errors
	if cumulativeErr != nil {
		return cumulativeErr
	} else {
		return nil
	}
}
