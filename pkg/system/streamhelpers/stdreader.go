package streamhelpers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"go.uber.org/zap"
)

const (
	GRACE_PERIOD_TIME_DEFAULT = 5 * time.Second
	WRITE_BUFFER_SIZE_DEFAULT = 65535
)

func timeout(c chan<- bool, timeout time.Duration) {
	time.Sleep(timeout)
	c <- true
}

type captureFile struct {
	path    string
	flags   int
	perm    fs.FileMode
	dirperm fs.FileMode
}

type CaptureFiles struct {
	StdOUT *captureFile
	StdERR *captureFile
}

type CaptureStreams struct {
	StdOUT io.Writer
	StdERR io.Writer
}

type ProcessStuckError struct {
	PID int
}

func (m *ProcessStuckError) Error() string {
	return fmt.Sprintf("process with pid %d was stuck", m.PID)
}

func (e *ProcessStuckError) Is(tgt error) bool {
	_, ok := tgt.(*ProcessStuckError)
	return ok
}

type ProcessNotStartedError struct {
	msg string
}

func (m *ProcessNotStartedError) Error() string {
	return m.msg
}

func (e *ProcessNotStartedError) Is(tgt error) bool {
	_, ok := tgt.(*ProcessNotStartedError)
	return ok
}

// This function gracefully terminates a process by sending SIGTERM first and then killing it
func (r *stdReader) GracefulTermination(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		// This emits done, as soon as the process exits
		done <- cmd.Wait()
	}()

	targetPID := cmd.Process.Pid
	if r.useProcessGroup {
		// Use the negative PID if we use a process group
		targetPID = -targetPID
	}

	select {
	case err := <-done:
		// Check how the process terminated
		if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				apglog.Error("process was terminated by outside signal: %v", zap.Int("pid", targetPID), zap.Any("signal", status.Signal()))
				return err
			}

			apglog.Info("process terminated okay", zap.Int("pid", targetPID), zap.Error(err))
			return nil
		}

		apglog.Error("process did not exit clearnly", zap.Error(err), zap.Int("pid", targetPID))
		return err
	// The context timeout is reached or the user requested cancellation
	case <-r.ctx.Done():
		if cmd.Process == nil {
			apglog.Info("process was not started yet")
			return fmt.Errorf("process not started yet")
		}

		// Save the string representation of the termination signal
		terminationSignalStr := r.terminationSignal.String()

		// Give it some grace-period after invoking the signal specified by the user
		apglog.Info("invoking signal", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr))
		err := syscall.Kill(targetPID, r.terminationSignal)
		if err != nil {
			apglog.Warn("could not send signal to process", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr), zap.Error(err))
			if errors.Is(err, syscall.ESRCH) {
				apglog.Panic("something is blocking cmd.Wait() from finishing")
			}
		}

		// If the user wanted to kill the process, we can skip the timeout handlin
		if r.terminationSignal == syscall.SIGKILL {
			// Kill is guaranteed to terminate, so this is safe
			apglog.Info("sigkill requested by user, process exited", zap.Int("pid", targetPID), zap.Error(err))

			// Wait needs to terminate correctly
			<-done
			return nil
		}

		// Start the async. timeout
		shutdownTimeoutReached := make(chan bool)
		go timeout(shutdownTimeoutReached, r.gracePeriod)
		apglog.Info("started exit graceperiod", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr))

		select {
		case err = <-done:
			apglog.Info("process finished after cancellation request", zap.Int("pid", targetPID), zap.Error(err))

			// We dont want these "fake" errors to bubble up bcs the process honored the request
			return nil
		case <-shutdownTimeoutReached:
			// If the process is still running, send a SIGKILL signal to force it to exit
			apglog.Warn("shutdown timeout reached, killing stuck process", zap.Int("pid", targetPID))

			// Time to say goodbye
			err = syscall.Kill(targetPID, syscall.SIGKILL)
			if err != nil {
				apglog.Error("error sending SIGKILL to process", zap.Int("pid", targetPID), zap.Error(err))

				// If the process is done but we still timed out, someone didnt close a stream or two"
				if errors.Is(err, syscall.ESRCH) {
					apglog.Panic("something is blocking cmd.Wait() from finishing")
				}
			}

			// Wait needs to terminate correctly
			<-done

			// Return error if the process was stuck
			return &ProcessStuckError{targetPID}
		}
	}
}

func NewCaptureFile(path string) *captureFile {
	var f captureFile
	f.path = path
	f.flags = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	f.perm = 0660
	f.dirperm = 0770
	return &f
}

func (f *captureFile) WithFlags(flags int) *captureFile {
	f.flags = flags
	return f
}

func (f *captureFile) WithPermissions(file fs.FileMode, dir fs.FileMode) *captureFile {
	f.perm = file
	f.dirperm = dir
	return f
}

type stdReader struct {
	files             *CaptureFiles
	streams           *CaptureStreams
	terminationSignal syscall.Signal
	useProcessGroup   bool

	// The context we have been started with
	ctx context.Context
	// The command that we are supposedt to run
	cmd *exec.Cmd

	// Store the dynamically assignable writers
	stdOutMultiWriter *DynamicMultiWriter
	stdErrMultiWriter *DynamicMultiWriter

	// The grace period to use
	gracePeriod         time.Duration
	fileWriteBufferSize int

	// Flag to determine if the user already called capture
	invoked bool
}

// This creates a new capture settings struct
// You can specify both files and streams or only one.
func NewSTDReader(cmd *exec.Cmd, ctx context.Context) *stdReader {
	return &stdReader{
		terminationSignal:   syscall.SIGTERM,
		useProcessGroup:     true,
		cmd:                 cmd,
		ctx:                 ctx,
		stdOutMultiWriter:   NewDynamicMultiWriter(),
		stdErrMultiWriter:   NewDynamicMultiWriter(),
		gracePeriod:         GRACE_PERIOD_TIME_DEFAULT,
		fileWriteBufferSize: WRITE_BUFFER_SIZE_DEFAULT,
		invoked:             false,
	}
}

// Add output files
func (c *stdReader) WithFiles(files CaptureFiles) *stdReader {
	c.files = &files
	return c
}

// Add streams that are always part of the systemthat are automatically closed by us
func (c *stdReader) WithStreams(streams CaptureStreams) *stdReader {
	c.streams = &streams
	return c
}

// Attach an arbitary writer to the given outputType, if you want to remove it use
// DetachStream(writer) to do so. Make sure to perform all closing operations yourself!
func (c *stdReader) AttachStream(outputType OutputType, writer io.Writer) *stdReader {
	c.appendWriterByType(outputType, writer)
	return c
}

// Only request the main pid of the process to terminate
// This is dangerous and might leave processes behind, only use when you know what you are doing
func (c *stdReader) SetTerminateMainOnly() *stdReader {
	c.useProcessGroup = false
	return c
}

// Use a custom graceful termination signal, some processes might need it to exit cleanly
func (c *stdReader) SetTerminationSignal(sig syscall.Signal) *stdReader {
	c.terminationSignal = sig
	return c
}

// Set the amount of time that has to pass before the process is killed if it did not
// respond to the termination signal.
func (c *stdReader) SetGracePeriod(period time.Duration) *stdReader {
	c.gracePeriod = period
	return c
}

// Set the write buffer size for the files specified
func (c *stdReader) SetFileWriteBufferSize(size int) *stdReader {
	if size < 1 {
		apglog.Panic("file write buffer too small", zap.Int("requested", size))
		return nil
	}

	c.fileWriteBufferSize = size
	return c
}

// Creates a file at the specified path
// Do not forget to call close on this!
func createFile(file *captureFile) (*os.File, error) {
	if _, err := os.Stat(file.path); os.IsNotExist(err) {
		dirPath, err := filepath.Abs(filepath.Dir(file.path))
		if err != nil {
			apglog.Error("failed to get absolute path", zap.String("path", dirPath))
			return nil, err
		}

		if err = os.MkdirAll(dirPath, file.dirperm); err != nil {
			apglog.Error("could not create required directories", zap.String("path", dirPath))
			return nil, err
		}
	}

	// Create the output file with restrictive permission
	outfile, err := os.OpenFile(file.path, file.flags, file.perm)
	if err != nil {
		apglog.Error("could not create output file", zap.String("file", file.path))
		return nil, err
	}

	return outfile, nil
}

type OutputType byte

const (
	STDOUT_OUT OutputType = iota
	STDERR_OUT
)

// This directly appends a writer for the given type, no closing is performed
func (r *stdReader) appendWriterByType(writerType OutputType, writer io.Writer) {
	if writerType == STDOUT_OUT {
		r.stdOutMultiWriter.Append(writer)
	} else if writerType == STDERR_OUT {
		r.stdErrMultiWriter.Append(writer)
	}
}

// Append the writer only if its not nil and return a closer if its closeable
func (r *stdReader) appendWriterIfSet(writerType OutputType, writer io.Writer) func() error {
	if writer == nil {
		return ErrNilFunc
	}

	// Append the writer to the writer list
	r.appendWriterByType(writerType, writer)

	return func() error {
		return CloseIfCloseable(writer)
	}
}

func (r *stdReader) appendCaptureFileWriterIfSet(writerType OutputType, file *captureFile) (func() error, error) {
	// Optional
	if file == nil {
		return ErrNilFunc, nil
	}

	// open the out file for writing
	outfile, err := createFile(file)
	if err != nil {
		return ErrNilFunc, err
	}

	// Prepare and assign target stream
	bufferedWriter := bufio.NewWriterSize(outfile, int(r.fileWriteBufferSize))

	// Append the writer to the writer list
	r.appendWriterByType(writerType, bufferedWriter)

	// This function takes care of flushing and closing the file
	return func() error {
		// Ignore flush errors
		bufferedWriter.Flush()
		return outfile.Close()
	}, nil
}

func (r *stdReader) Capture() error {
	// Sanity check if the user already invoked us by accident
	if r.invoked {
		log.Panic("already running, undefined behavior, abort")
		return fmt.Errorf("capture can not be run twice")
	}

	// Signal that we ran already
	r.invoked = true

	apglog.Debug("preparing command execution", zap.String("cmd", r.cmd.String()))

	// Assign the streams the user specified and close them if we finish
	if r.streams != nil {
		closeStream := r.appendWriterIfSet(STDOUT_OUT, r.streams.StdOUT)
		defer closeStream()

		closeStream = r.appendWriterIfSet(STDERR_OUT, r.streams.StdERR)
		defer closeStream()
	}

	// Next check the files
	if r.files != nil {
		closeFile, err := r.appendCaptureFileWriterIfSet(STDOUT_OUT, r.files.StdOUT)
		if err != nil {
			return err
		}

		// If we have a file and need to close it, do so later
		defer closeFile()

		// Do the same for stderr
		closeFile, err = r.appendCaptureFileWriterIfSet(STDERR_OUT, r.files.StdERR)
		if err != nil {
			return err
		}

		defer closeFile()
	}

	// Default outputs to /dev/null
	r.cmd.Stdout = nil
	r.cmd.Stderr = nil

	// Even if its only one writer, we can use MultiWriter here
	if size := r.stdOutMultiWriter.Size(); size > 0 {
		r.cmd.Stdout = r.stdOutMultiWriter
		apglog.Debug("assigned stdout writers", zap.Int("count", size))
	}

	if size := r.stdErrMultiWriter.Size(); size > 0 {
		r.cmd.Stderr = r.stdErrMultiWriter
		apglog.Debug("assigned stderr writers", zap.Int("count", size))
	}

	// Sanity check that we dont over-use this function
	if r.cmd.Stdout == nil && r.cmd.Stderr == nil {
		apglog.Warn("no output selected on reader that is designed to output things, wrong function?")
	}

	// This requests a process group from the system, all spawned children will belong to it
	if r.useProcessGroup {
		r.cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	// Start the process
	err := r.cmd.Start()
	if err != nil {
		apglog.Error("could not start process", zap.Error(err))

		// Delete the files if the process did not run
		if stdOutF := r.files.StdOUT; stdOutF != nil {
			os.Remove(stdOutF.path)
		}
		if stdErrF := r.files.StdERR; stdErrF != nil {
			os.Remove(stdErrF.path)
		}

		return &ProcessNotStartedError{err.Error()}
	}

	// Start termination handler
	return r.GracefulTermination(r.cmd)
}

// Detach an active writer
func (r *stdReader) DetachStream(writer io.Writer, close bool) bool {
	// Dont permit removing writers added using WithStreams, as we close them
	if r.streams != nil &&
		(r.streams.StdERR == writer || r.streams.StdOUT == writer) {
		apglog.Panic("can not detach a fixed stream, use AttachStream instead of WithStreams")
		return false
	}

	// User is relying on us to close it
	if close {
		defer CloseIfCloseable(writer)
	}

	// Try to remove the dynamically attached writer
	if !r.stdOutMultiWriter.Remove(writer) && !r.stdErrMultiWriter.Remove(writer) {
		apglog.Error("cant detach writer, not found")
		return false
	}

	return true
}
