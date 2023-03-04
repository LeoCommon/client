package iridium

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/streamhelpers"
	"disco.cs.uni-kl.de/apogee/pkg/test"
	"github.com/stretchr/testify/assert"
)

var (
	SCRIPT_DIR string = test.GetScriptPath("iridium")
	TMP_DIR    string
	TEST_START time.Time
)

func SetupIridiumTest(t *testing.T) func() {
	log.Init(true)
	TMP_DIR = t.TempDir()

	// Change to the scripts directory
	os.Chdir(SCRIPT_DIR)
	os.Setenv("PATH", os.Getenv("PATH")+":"+SCRIPT_DIR)

	TEST_START = time.Now()

	// shared tear down logic, if any
	return func() {}
}

type FixedJob struct {
	Id        string            `json:"id"`
	Name      string            `json:"name"`
	StartTime int64             `json:"start_time"`
	EndTime   int64             `json:"end_time"`
	Command   string            `json:"command"`
	Arguments map[string]string `json:"arguments"`
	Sensors   []string          `json:"sensors"`
	Status    string            `json:"status"`
	States    map[string]string `json:"states"`
}

func TestIridiumSniffingEarlyExit(t *testing.T) {
	SetupIridiumTest(t)

	app, err := client.Setup(true)
	assert.NoError(t, err)

	app.Config.Client.Jobs.StoragePath = TMP_DIR + "/jobs/"
	app.Config.Client.Jobs.TempPath = TMP_DIR

	err = IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      "testing_iridium_extractor",
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + 10,
	}, app)

	assert.NoError(t, err)
}

func TestIridiumSniffing(t *testing.T) {
	// Change to the realtime directory
	SCRIPT_DIR += "realtime/"
	SetupIridiumTest(t)

	app, err := client.Setup(true)
	assert.NoError(t, err)

	app.Config.Client.Jobs.StoragePath = TMP_DIR + "/jobs/"
	app.Config.Client.Jobs.TempPath = TMP_DIR

	// This test needs a mock api to work, until then we check if it panics at the end
	assert.Panics(t, func() {
		IridiumSniffing(api.FixedJob{
			Id:        "mock_test",
			Name:      "testing_iridium_extractor",
			StartTime: time.Now().Unix(),
			EndTime:   time.Now().Unix() + 10,
		}, app)
	})
}

func TestIridiumIntegrationTimeout(t *testing.T) {
	defer SetupIridiumTest(t)()

	var wg sync.WaitGroup

	cmd := exec.Command("./iridium-extractor", "-D", "4", "fakeconfigFile.conf")
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	sniffingOutput := streamhelpers.NewCaptureFile(TMP_DIR + "stdout.log")
	errorLog := streamhelpers.NewCaptureFile(TMP_DIR + "stderr.log")

	pre, pwo := io.Pipe()
	pro, pwe := io.Pipe()

	defer pre.Close()
	defer pro.Close()

	// This is blocking here
	stdReader := streamhelpers.NewSTDReader(cmd, ctx).
		WithFiles(streamhelpers.CaptureFiles{
			StdOUT: sniffingOutput,
			StdERR: errorLog,
		}).
		SetTerminationSignal(syscall.SIGINT).
		AttachStream(streamhelpers.STDERR_OUT, pwe).
		AttachStream(streamhelpers.STDOUT_OUT, pwo)

	wg.Add(1)
	go func() {
		assert.NoError(t, stdReader.Capture())
		wg.Done()
	}()

	scanner := bufio.NewScanner(pro)
	wg.Add(1)
	go func() {
		startLineFound := false
		for scanner.Scan() {
			m := scanner.Text()

			if strings.Contains(m, "Using HackRF One with firmware") {
				startLineFound = true
			}
		}

		// Check if the start line was read
		assert.True(t, startLineFound)

		// The pipe was disconnected
		log.Info("pipe/writer/stream disconnected as requested")
		wg.Done()
	}()

	stdErrScanner := bufio.NewScanner(pre)
	wg.Add(1)
	go func() {
		lineFound := false
		for stdErrScanner.Scan() {
			m := stdErrScanner.Text()

			if strings.Contains(m, "RAW") {
				lineFound = true
			}
		}

		// Check if the output line was read
		assert.True(t, lineFound)

		// The pipe was disconnected
		log.Info("pipe/writer/stream disconnected as requested")
		wg.Done()
	}()

	// Wait for the initial troubleshooting to complete
	time.Sleep(time.Millisecond * 100)

	// Detach the stream and close it
	assert.True(t, stdReader.DetachStream(pwo))
	assert.NoError(t, pwo.Close())

	assert.True(t, stdReader.DetachStream(pwe))
	assert.NoError(t, pwe.Close())

	log.Info("Troubleshooter exited")

	wg.Wait()

	// The original termination time 250ms + 50ms
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*300)
}
