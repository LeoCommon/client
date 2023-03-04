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
	t.Helper()

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
	t.Helper()
	assert.FileExists(t, STDOUT_FILE)
	assert.FileExists(t, STDERR_FILE)
}

func VerifyNoOutputFiles(t *testing.T) {
	t.Helper()
	assert.NoFileExists(t, STDOUT_FILE)
	assert.NoFileExists(t, STDERR_FILE)
}

func VerifyStreamsAndCloseReaders(t *testing.T, stdoPR *io.PipeReader, stdePR *io.PipeReader) {
	t.Helper()

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
	t.Helper()
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
	reader.SetGracePeriod(time.Millisecond * 10)

	// Stuck process should return error
	assert.ErrorIs(t, reader.Capture(), &ProcessStuckError{})

	// Check that the output file was created
	VerifyOutputFiles(t)

	// Check that we terminated in time 50(request) + 10 (grace) + delta
	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*80)

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

	// But run in sync, if we dont read these streams, stdreader should recover them for us
	assert.NotPanics(t, func() { reader.Capture() })

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

	assert.Panics(t, func() {
		reader.DetachStream(stdoPW)
	})

	assert.NoError(t, stdoPW.Close())
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
	SetupStdReaderTest(t)

	cmd := exec.Command("./output.sh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	pro, pwo := io.Pipe()
	pre, pwe := io.Pipe()

	stdReader := NewSTDReader(cmd, ctx).
		SetTerminationSignal(syscall.SIGINT).
		WithStreams(CaptureStreams{
			StdOUT: pwo,
			StdERR: pwe,
		})

	wg := &sync.WaitGroup{}

	// We would encounter EOF on the scanner, but scanner.scan supresses that
	go runStreamTest(t, wg, pro, "date", "error", nil, nil, 1)
	go runStreamTest(t, wg, pre, "error", "date", nil, nil, 1)

	// Wait for the process to finish
	assert.NoError(t, stdReader.Capture())

	// This should instantly return
	wg.Wait()

	// Verify that our readers are returning EOF
	VerifyStreamsAndCloseReaders(t, pro, pre)

	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*200)
}

func TestStreamsEarlyDetach(t *testing.T) {
	SetupStdReaderTest(t)

	cmd := exec.Command("./output.sh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	pro, pwo := io.Pipe()
	pre, pwe := io.Pipe()

	stdReader := NewSTDReader(cmd, ctx).
		SetTerminationSignal(syscall.SIGINT).SetGracePeriod(time.Millisecond * 100)

	// Attach the streams
	stdReader.AttachStream(STDERR_OUT, pwe)
	stdReader.AttachStream(STDOUT_OUT, pwo)

	// Detach the streams
	assert.True(t, stdReader.DetachStream(pwo))
	assert.True(t, stdReader.DetachStream(pwe))
	assert.NoError(t, pwo.Close())
	assert.NoError(t, pwe.Close())

	// Start the capture
	wg := &sync.WaitGroup{}
	go runStreamTest(t, wg, pro, "", "", io.EOF, nil, 0)
	go runStreamTest(t, wg, pre, "", "", io.EOF, nil, 0)

	wg.Add(1)
	go func() {
		assert.NoError(t, stdReader.Capture())
		wg.Done()
	}()

	wg.Wait()

	// Verify that our readers are returning EOF
	VerifyStreamsAndCloseReaders(t, pro, pre)

	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*450)
}

func TestStreamsDetachWhileRunning(t *testing.T) {
	SetupStdReaderTest(t)

	cmd := exec.Command("./output.sh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	pro, pwo := io.Pipe()
	pre, pwe := io.Pipe()

	stdReader := NewSTDReader(cmd, ctx).
		SetTerminationSignal(syscall.SIGINT).SetGracePeriod(time.Millisecond * 100)

	// Attach the streams
	stdReader.AttachStream(STDERR_OUT, pwe)
	stdReader.AttachStream(STDOUT_OUT, pwo)

	// Start the capture
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		assert.NoError(t, stdReader.Capture())
		wg.Done()
	}()

	// Scanner is EOF but this is not considered an error
	go runStreamTest(t, wg, pro, "date", "error", nil, nil, 1)
	go runStreamTest(t, wg, pre, "error", "date", nil, nil, 1)

	time.Sleep(100 * time.Millisecond)

	// Detach the streams
	assert.True(t, stdReader.DetachStream(pwo))
	assert.True(t, stdReader.DetachStream(pwe))
	assert.NoError(t, pwo.Close())
	assert.NoError(t, pwe.Close())

	// Wait the full 200ms
	wg.Wait()

	// Verify that our readers are returning EOF
	VerifyStreamsAndCloseReaders(t, pro, pre)

	assert.WithinDuration(t, TEST_START, time.Now(), time.Millisecond*150)
}

func runStreamTest(t *testing.T, wg *sync.WaitGroup, stream io.Reader, contains, notContains string, readError error, scannerError error, minOutputLines int) {
	t.Helper()

	wg.Add(1)
	defer wg.Done()

	var byteb []byte
	_, err := stream.Read(byteb)
	assert.ErrorIs(t, err, readError)

	// Create scanner
	scanner := bufio.NewScanner(stream)
	runs := 0
	for scanner.Scan() {
		text := scanner.Text()
		if contains != "" {
			assert.Contains(t, text, contains)
		}
		if notContains != "" {
			assert.NotContains(t, text, notContains)
		}
		runs++
	}

	// Check the resulting error for consistency
	assert.ErrorIs(t, scanner.Err(), scannerError)

	// Check if we ran a minimum amount of times
	assert.GreaterOrEqual(t, runs, minOutputLines)
}
