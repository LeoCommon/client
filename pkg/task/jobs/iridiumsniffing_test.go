package jobs

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

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
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
	apglog.Init(true)
	TMP_DIR = t.TempDir()

	// Change to the scripts directory
	os.Chdir(SCRIPT_DIR)
	TEST_START = time.Now()

	// shared tear down logic, if any
	return func() {}
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
		apglog.Info("pipe/writer/stream disconnected as requested")
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
		apglog.Info("pipe/writer/stream disconnected as requested")
		wg.Done()
	}()

	// Wait for the initial troubleshooting to complete
	time.Sleep(time.Millisecond * 100)

	// Detach the stream and close it
	assert.True(t, stdReader.DetachStream(pwo, true))
	assert.True(t, stdReader.DetachStream(pwe, true))

	apglog.Info("Troubleshooter exited")

	wg.Wait()

	// The original termination time 250ms + 50ms
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*300)
}
