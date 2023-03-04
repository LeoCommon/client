package streamhelpers

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/test"
	"github.com/stretchr/testify/assert"
)

var (
	SCRIPT_DIR            string = test.GetScriptPath("stdreader")
	TMP_DIR               string
	STDOUT_FILE           string
	STDERR_FILE           string
	DEFAULT_CAPTURE_FILES CaptureFiles
	TEST_START            time.Time
)

func SetupStdReaderTest(t *testing.T) func() {
	log.Init(true)
	TMP_DIR = t.TempDir()
	STDOUT_FILE = TMP_DIR + "/out.file"
	STDERR_FILE = TMP_DIR + "/err.file"

	// Might have been modified by a caller to re-use tests
	DEFAULT_CAPTURE_FILES = CaptureFiles{
		StdOUT: NewCaptureFile(STDOUT_FILE),
		StdERR: NewCaptureFile(STDERR_FILE),
	}

	// Change to the scripts directory
	os.Chdir(SCRIPT_DIR)
	TEST_START = time.Now()

	// shared tear down logic, if any
	return func() {}
}

// Check that the output files were created
func VerifyOutputFiles(t *testing.T) {
	assert.FileExists(t, STDOUT_FILE)
	assert.FileExists(t, STDERR_FILE)
}

func VerifyNoOutputFiles(t *testing.T) {
	assert.NoFileExists(t, STDOUT_FILE)
	assert.NoFileExists(t, STDERR_FILE)
}

func VerifyStreamsAndCloseReaders(t *testing.T, stdoPR *io.PipeReader, stdePR *io.PipeReader) {
	// Check that the readers return EOF
	var buf []byte
	_, err := stdoPR.Read(buf)
	assert.ErrorIs(t, err, io.EOF)

	_, err = stdePR.Read(buf)
	assert.ErrorIs(t, err, io.EOF)

	assert.NoError(t, stdoPR.Close())
	assert.NoError(t, stdePR.Close())
}

func BasicFileTerminationRun(t *testing.T) {
	// We let the program terminate itself, max wait is 1 second
	ctx, intCancel := context.WithTimeout(context.Background(), time.Second)
	defer intCancel()

	// Run the sleep script for 50 milliseconds
	reader := NewSTDReader(exec.Command("./sleep.sh", "0.05"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)

	// Block but return no error, normal exit
	assert.NoError(t, reader.Capture())

	// The context should not have terminated
	assert.NoError(t, ctx.Err())

	// Test if the sleep.sh terminated correctly
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*100)
}

func TestNormalTermination(t *testing.T) {
	defer SetupStdReaderTest(t)()

	BasicFileTerminationRun(t)
	VerifyOutputFiles(t)
}

func TestNilFiles(t *testing.T) {
	defer SetupStdReaderTest(t)()

	DEFAULT_CAPTURE_FILES = CaptureFiles{StdOUT: nil, StdERR: nil}
	BasicFileTerminationRun(t)
	VerifyNoOutputFiles(t)
}

func TestPanicOnDoubleCapture(t *testing.T) {
	defer SetupStdReaderTest(t)()

	// We let the program terminate itself, max wait is 1 second
	ctx, intCancel := context.WithTimeout(context.Background(), time.Second)
	defer intCancel()

	// Run the sleep script
	reader := NewSTDReader(exec.Command("./sleep.sh", "0"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)

	// First run, no error
	assert.NoError(t, reader.Capture())
	// Panic on second attempt
	assert.Panics(t, func() { reader.Capture() })
}

func TestInvalidCommand(t *testing.T) {
	defer SetupStdReaderTest(t)()

	// We let the program terminate itself, max wait is 1 second
	ctx, intCancel := context.WithTimeout(context.Background(), time.Second)
	defer intCancel()

	// Run a command that does not exist
	reader := NewSTDReader(exec.Command("./we-dont-exist.sh"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)

	// First run, no error
	assert.ErrorIs(t, reader.Capture(), &ProcessNotStartedError{})

	// If the process did not even start we dont need files to fly around here
	VerifyNoOutputFiles(t)
}

func TestTimeout(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./sleep.sh", "0.2"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)

	// Block but return no error, normal exit
	assert.NoError(t, reader.Capture())

	// Check that we terminated in time
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*100)

	// Check that the output file was created
	VerifyOutputFiles(t)

	// The context should not have terminated
	assert.Error(t, ctx.Err())
}

func TestCancel(t *testing.T) {
	defer SetupStdReaderTest(t)()

	var wg sync.WaitGroup

	ctx, intCancel := context.WithTimeout(context.Background(), time.Second)

	wg.Add(1)
	go func() {
		// Sleep for 500ms
		reader := NewSTDReader(exec.Command("./sleep.sh", "0.50"), ctx)
		reader.WithFiles(DEFAULT_CAPTURE_FILES)

		// Cancel should not invoke any error
		assert.NoError(t, reader.Capture())
		wg.Done()
	}()

	// Wait for 50 milliseconds
	time.Sleep(time.Duration(50 * time.Millisecond))

	// Cancel the context
	intCancel()

	// Wait for termination, should trigger instantly
	wg.Wait()

	// Check that the output file was created
	VerifyOutputFiles(t)

	// Check that we terminated in time
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*100)

	// The context should have terminated
	assert.Error(t, ctx.Err())
}

func TestIgnoreSigint(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./trap.sh"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)
	reader.SetTerminationSignal(syscall.SIGINT)

	// Reduce the default grace period
	reader.SetGracePeriod(time.Millisecond * 50)

	// Stuck process should return error
	assert.ErrorIs(t, reader.Capture(), &ProcessStuckError{})

	// Check that the output file was created
	VerifyOutputFiles(t)

	// Check that we terminated in time 50(request) + 50 (grace) + delta
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*150)

	// The context should have terminated
	assert.Error(t, ctx.Err())
}

func TestForceKill(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./trap.sh"), ctx)
	reader.WithFiles(DEFAULT_CAPTURE_FILES)

	// Set the grace period, but by sending SIGKILL we skip it
	reader.SetGracePeriod(time.Millisecond * 100)
	// Force terminate the script
	reader.SetTerminationSignal(syscall.SIGKILL)

	// Nothing can be stuck if we kill it, so no error!
	assert.NoError(t, reader.Capture())

	// Check that the output file was created
	VerifyOutputFiles(t)

	// Check that we terminated in time 50(request) + delta
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*100)

	// The context should have terminated
	assert.Error(t, ctx.Err())
}

func TestStreamDeadlockingWait(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./output.sh"), ctx)
	reader.SetGracePeriod(time.Millisecond * 20)

	// Create two pipes
	stdoPR, stdoPW := io.Pipe()
	stdePR, stdePW := io.Pipe()

	// Run with streams
	reader.WithStreams(CaptureStreams{
		StdOUT: stdoPW,
		StdERR: stdePW,
	})

	// But run in sync, if we dont read these streams, we will dead-lock
	assert.Panics(t, func() { reader.Capture() })

	// Check that we terminated in time 50(request) + 20 (grace) + delta
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*100)

	// The context should have terminated
	assert.Error(t, ctx.Err())

	VerifyStreamsAndCloseReaders(t, stdoPR, stdePR)
}

func TestStreamsInvalidRemoval(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./output.sh"), ctx)
	reader.SetGracePeriod(time.Millisecond * 100)

	// Create two pipes
	_, stdoPW := io.Pipe()

	// Run with streams
	reader.WithStreams(CaptureStreams{
		StdOUT: stdoPW,
	})

	assert.Panics(t, func() { reader.DetachStream(stdoPW, true) })
}

func TestInvalidWriteBufferSize(t *testing.T) {
	defer SetupStdReaderTest(t)()

	ctx, intCancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer intCancel()

	reader := NewSTDReader(exec.Command("./output.sh"), ctx)

	assert.Panics(t, func() { reader.SetFileWriteBufferSize(0) })
}

// This test case covers quite a bit of everything stream related
func TestStreams(t *testing.T) {
	defer SetupStdReaderTest(t)()

	var wg sync.WaitGroup

	cmd := exec.Command("./output.sh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	pro, pwo := io.Pipe()
	pre, pwe := io.Pipe()

	// This is blocking here
	stdReader := NewSTDReader(cmd, ctx).
		SetTerminationSignal(syscall.SIGINT).
		AttachStream(STDERR_OUT, pwe).
		AttachStream(STDOUT_OUT, pwo)

	wg.Add(1)
	go func() {
		assert.NoError(t, stdReader.Capture())
		wg.Done()
	}()

	// Start a goroutine to run the loop.
	stdoscan := bufio.NewScanner(pro)
	wg.Add(1)
	go func() {
		runs := 0
		for stdoscan.Scan() {
			assert.Contains(t, stdoscan.Text(), "date")
			assert.NotContains(t, stdoscan.Text(), "error")
			runs++
		}

		// Minimum runs should be one
		assert.GreaterOrEqual(t, runs, 1)

		// The pipe was disconnected
		wg.Done()
	}()

	stdescan := bufio.NewScanner(pre)
	wg.Add(1)
	go func() {
		runs := 0
		for stdescan.Scan() {
			assert.Contains(t, stdescan.Text(), "error")
			assert.NotContains(t, stdescan.Text(), "date")
			runs++
		}

		// Minimum runs should be one
		assert.GreaterOrEqual(t, runs, 1)

		// The pipe was disconnected
		wg.Done()
	}()

	// Start a timer that will fire after 200 milliseconds
	time.Sleep(time.Millisecond * 50)

	// Detach the streams and tell stdreader to close it
	assert.True(t, stdReader.DetachStream(pwo, true))
	assert.True(t, stdReader.DetachStream(pwe, true))

	// Verify that our readers are returning EOF
	VerifyStreamsAndCloseReaders(t, pro, pre)

	wg.Wait()

	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*350)
}
