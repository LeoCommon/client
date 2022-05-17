package jobHandler

import (
	"strconv"
	"strings"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/api"
)

var pollingInterval int64 = 10 //seconds
var pendingJobQueue []api.FixedJob
var runningJobs []api.FixedJob

func findJobInList(findId string, jobList []api.FixedJob) int {
	for i := 0; i < len(jobList); i++ {
		if jobList[i].Id == findId {
			return i
		}
	}
	return -1
}

func executeTheJob(myJob api.FixedJob) {
	//wait until the job is due
	creationTime := time.Now().Unix()
	waitTime := myJob.StartTime - creationTime
	time.Sleep(time.Duration(waitTime) * time.Second)
	//set the job online to running
	err := api.PutJobUpdate(myJob.Name, "running")
	if err != nil {
		apglog.Info("Put Job " + myJob.Name + " into 'running' did not work: " + err.Error())
	}
	//execute the job
	startExecTime := time.Now().Unix()
	myCommand := strings.ToLower(myJob.Command)
	allRight := true
	if strings.Contains("get_status, push_status, return_status, small_status", myCommand) {
		err := PushStatus()
		if err != nil {
			apglog.Error("Error during pushing a status: " + err.Error())
			allRight = false
		}
	} else if strings.Contains("get_full_status, get_verbose_status, get_big_status", myCommand) {
		err := ReportFullStatus(myJob.Name)
		if err != nil {
			apglog.Error("Error during pushing a full status: " + err.Error())
			allRight = false
		}
	} else if strings.Contains("upload_test_file, test_txt", myCommand) {
		filetext := "some text to write." +
			"\njob-schedule-time:" + strconv.FormatInt(creationTime, 10) +
			"\njob-exec-time:" + strconv.FormatInt(startExecTime, 10)
		err := UploadTestFile(myJob.Name, filetext)
		if err != nil {
			apglog.Error("Error during pushing a full status: " + err.Error())
			allRight = false
		}
	} else if strings.Contains("iridium_sniffing, iridiumSniffing", myCommand) {
		err := IridiumSniffing(myJob)
		if err != nil {
			apglog.Error("Error during iridium-sniffing: " + err.Error())
			allRight = false
		}
	} else {
		apglog.Info("Ignoring job " + myJob.Name + " (Id:" + myJob.Id + ") with unknown command: " + myCommand)
	}
	// finally send a job status
	if allRight {
		err = api.PutJobUpdate(myJob.Name, "finished")
		if err != nil {
			apglog.Info("Put Job " + myJob.Name + " into 'finished' did not work: " + err.Error())
		}
	} else {
		err = api.PutJobUpdate(myJob.Name, "failed")
		if err != nil {
			apglog.Info("Put Job " + myJob.Name + " into 'failed' did not work: " + err.Error())
		}
	}
	// remove your job from the running list
	index := findJobInList(myJob.Id, runningJobs)
	runningJobs = append(runningJobs[:index], runningJobs[index+1:]...)
}

func HandleNewJobs(jobs []api.FixedJob) {
	// when a new job-list was pulled from the server
	myTime := time.Now().Unix()
	nextPollingTime := myTime + pollingInterval
	tempPendingJobs := []api.FixedJob{}
	for i := 0; i < len(jobs); i++ {
		tempJob := jobs[i]
		if tempJob.EndTime > myTime && tempJob.StartTime < tempJob.EndTime {
			if tempJob.StartTime < myTime {
				// if a job should be running, check for that
				runningIndex := findJobInList(tempJob.Id, runningJobs)
				if runningIndex == -1 {
					// job was not fund, so start it
					runningJobs = append(runningJobs, tempJob)
					apglog.Info("Start job that should be running already:" + tempJob.Name)
					go executeTheJob(tempJob)
				}
			} else if tempJob.StartTime < nextPollingTime {
				// start go-routines for all pending jobs in the next polling_interval (x minutes)
				runningJobs = append(runningJobs, tempJob)
				apglog.Info("Start upcoming job:" + tempJob.Name)
				go executeTheJob(tempJob)
			} else {
				// put pending jobs in temp pending list
				tempPendingJobs = append(tempPendingJobs, tempJob)
				apglog.Info("Enqueue pending job:" + tempJob.Name)
			}
			// replace old pending list by new pending list
			pendingJobQueue = tempPendingJobs
		} else {
			apglog.Info("Ignoring invalid job:" + tempJob.Name)
		}
	}
}

func HandleOldJobs() {
	// when no new job-list was pulled from the server (maybe no internet connection)
	myTime := time.Now().Unix()
	nextPollingTime := myTime + pollingInterval
	for i := 0; i < len(pendingJobQueue); i++ {
		tempJob := pendingJobQueue[i]
		if tempJob.StartTime < myTime {
			// if a job should be running, check for that
			runningIndex := findJobInList(tempJob.Id, runningJobs)
			if runningIndex == -1 {
				// job was not fund, so start it
				runningJobs = append(runningJobs, tempJob)
				apglog.Info("Start job that should be running already:" + tempJob.Name)
				go executeTheJob(tempJob)
			}
		} else if tempJob.StartTime < nextPollingTime {
			// start go-routines for all pending jobs in the next polling_interval (x minutes)
			runningJobs = append(runningJobs, tempJob)
			index := findJobInList(tempJob.Id, pendingJobQueue)
			pendingJobQueue = append(pendingJobQueue[:index], pendingJobQueue[index+1:]...)
			apglog.Info("Start upcoming job:" + tempJob.Name)
			go executeTheJob(tempJob)
		}
	}
}
