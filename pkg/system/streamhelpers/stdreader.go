package streamhelpers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/LeoCommon/client/pkg/log"
	"go.uber.org/zap"
)

const (
	// GracePeriodTimeDefault specifies the grace period, defaults to 10 seconds
	GracePeriodTimeDefault = 10 * time.Second

	// FileWriteBufferDefaultSize default write buffer size for files: 64 KiB
	FileWriteBufferDefaultSize = 65536
)

type CaptureFile struct {
	path    string
	flags   int
	perm    fs.FileMode
	dirperm fs.FileMode
}

type CaptureFiles struct {
	StdOUT *CaptureFile
	StdERR *CaptureFile
}

type CaptureStreams struct {
	StdOUT io.Writer
	StdERR io.Writer
}

type ProcessStuckError struct {
	PID int
}

func (e *ProcessStuckError) Error() string {
	return fmt.Sprintf("process with pid %d was stuck", e.PID)
}

func (e *ProcessStuckError) Is(err error) bool {
	_, ok := err.(*ProcessStuckError)
	return ok
}

type ProcessNotStartedError struct {
	msg string
}

func (e *ProcessNotStartedError) Error() string {
	return e.msg
}

func (e *ProcessNotStartedError) Is(err error) bool {
	_, ok := err.(*ProcessNotStartedError)
	return ok
}

func NewTerminatedEarlyError(err error) error {
	return &TerminatedEarlyError{err}
}

type TerminatedEarlyError struct {
	err error
}

func (e *TerminatedEarlyError) Error() string {
	if e.err == nil {
		return "no underlying error, exited fine"
	}

	return e.err.Error()
}

func (e *TerminatedEarlyError) Is(err error) bool {
	_, ok := err.(*TerminatedEarlyError)
	return ok
}

// Closes and detaches all streams we have available
// This returns true if at least one stream was closed
func (r *StdReader) closeEverything(err *error) {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		// First detach the streams without removing them from the MultiWriters (bulk mode)
		for k := range r.streamMap {
			r.detachStreamInternal(k, true)
		}

		// Now close all the files
		for _, fileCloser := range r.fileClosers {
			if *fileCloser != nil {
				closeError := (*fileCloser)(err)
				if closeError != nil {
					log.Debug("error closing file", zap.Error(closeError))
				}
			}
		}

		// Reset the multiwriters now
		r.stdErrMultiWriter.Reset()
		r.stdOutMultiWriter.Reset()
	})
}

// GracefulTermination gracefully terminates a process by sending
// SIGTERM first and then killing it Nothing in here needs a mutex, because we
// prevent changes once invoked is set to true
func (r *StdReader) GracefulTermination(cmd *exec.Cmd) error {
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
				log.Error("process was terminated by outside signal: %v", zap.Int("pid", targetPID), zap.Any("signal", status.Signal()))
				return err
			}

			log.Info("process terminated okay", zap.Int("pid", targetPID), zap.Error(err))
			return nil
		}

		log.Error("process did not exit cleanly", zap.Error(err), zap.Int("pid", targetPID))
		return err
	// The context timeout is reached or the user requested cancellation
	case <-r.ctx.Done():
		if cmd.Process == nil {
			log.Info("process was not started yet")
			return fmt.Errorf("process not started yet")
		}

		// Save the string representation of the termination signal
		terminationSignalStr := r.terminationSignal.String()

		// Give it some grace-period after invoking the signal specified by the user
		log.Info("invoking signal", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr))
		err := syscall.Kill(targetPID, r.terminationSignal)
		if err != nil {
			log.Error("could not send signal to process", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr), zap.Error(err))
		}

		// If the user wanted to kill the process, we can skip the timeout handlin
		if r.terminationSignal == syscall.SIGKILL {
			// Kill is guaranteed to terminate, so this is safe
			log.Info("sigkill requested by user, process exited", zap.Int("pid", targetPID), zap.Error(err))

			// Close everything to prevent a deadlock there is nothing that could be output anyway
			r.closeEverything(nil)

			log.Error("waiting for cmd.wait")
			// Wait for cmd.Wait() to terminate
			<-done
			return nil
		}

		// Start the async. timeout
		log.Info("start exit graceperiod", zap.Int("pid", targetPID), zap.String("signal", terminationSignalStr))

		select {
		case err = <-done:
			log.Info("process finished after cancellation request", zap.Int("pid", targetPID), zap.Error(err))

			// We dont want these "fake" errors to bubble up bcs the process honored the request
			return nil
		case <-time.After(r.gracePeriod):
			// If the process is still running, send a SIGKILL signal to force it to exit
			log.Warn("shutdown timeout reached, killing stuck process", zap.Int("pid", targetPID))

			// Close streams to prevent a deadlock there is nothing that could be output anyway
			r.closeEverything(nil)

			log.Error("delivering sigkill")

			// Time to say goodbye
			err = syscall.Kill(targetPID, syscall.SIGKILL)
			if err != nil {
				log.Error("error sending SIGKILL to process", zap.Int("pid", targetPID), zap.Error(err))
			}

			// Wait for cmd.Wait() to terminate
			log.Error("waiting for cmd.wait")
			<-done

			// Return error if the process was stuck
			return &ProcessStuckError{targetPID}
		}
	}
}

func NewCaptureFile(path string) *CaptureFile {
	var f CaptureFile
	f.path = path
	f.flags = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	f.perm = 0660
	f.dirperm = 0770
	return &f
}

func (f *CaptureFile) WithFlags(flags int) *CaptureFile {
	f.flags = flags
	return f
}

func (f *CaptureFile) WithPermissions(file fs.FileMode, dir fs.FileMode) *CaptureFile {
	f.perm = file
	f.dirperm = dir
	return f
}

type StdReader struct {
	// Make sure we close the stream only once
	closeOnce sync.Once
	mu        sync.RWMutex

	// The context we have been started with
	ctx context.Context

	// Store the dynamically assignable writers
	stdErrMultiWriter *DynamicMultiWriter
	// The command that we are supposedt to run
	cmd               *exec.Cmd
	stdOutMultiWriter *DynamicMultiWriter
	// Keep a copy of the stream list so we can auto-detach them
	streamMap map[io.Writer]bool

	// The processExitChannel channel signaling that the run is over
	processExitChannel chan error

	// All the file closer functions we need to run
	fileClosers CloseFuncPointers

	terminationSignal syscall.Signal
	// The grace period to use
	gracePeriod time.Duration

	fileWriteBufferSize int
	// Specifies if output files should be kept, even if the process did not start correctly or the size was 0
	alwaysKeepFiles bool

	useProcessGroup bool

	// Flag to determine if the user already called capture
	invoked bool
}

// NewSTDReader creates a new capture settings struct
// You can specify both files and streams or only one.
func NewSTDReader(cmd *exec.Cmd, ctx context.Context) *StdReader {
	return &StdReader{
		terminationSignal:   syscall.SIGTERM,
		useProcessGroup:     true,
		alwaysKeepFiles:     false,
		cmd:                 cmd,
		ctx:                 ctx,
		stdOutMultiWriter:   NewDynamicMultiWriter(),
		stdErrMultiWriter:   NewDynamicMultiWriter(),
		gracePeriod:         GracePeriodTimeDefault,
		fileWriteBufferSize: FileWriteBufferDefaultSize,
		invoked:             false,
		processExitChannel:  make(chan error, 1),
		streamMap:           make(map[io.Writer]bool),
	}
}

// WithFiles Add output files
func (r *StdReader) WithFiles(files CaptureFiles) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Not permitted
	if r.invoked {
		log.Panic("files can not be changed after they have been assigned once")
	}

	stdOutCloser, err := r.appendCaptureFileWriterIfSet(StdoutOut, files.StdOUT)
	if err != nil {
		return err
	}

	// Do the same for stderr
	stdErrCloser, err := r.appendCaptureFileWriterIfSet(StderrOut, files.StdERR)
	if err != nil {
		return err
	}

	// Append the file closers
	r.fileClosers = append(r.fileClosers, &stdOutCloser, &stdErrCloser)

	return nil
}

// WithStreams Add streams that are always part of the systemthat are
// automatically closed by us
func (r *StdReader) WithStreams(streams CaptureStreams) *StdReader {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Not permitted
	if r.invoked {
		log.Panic("streams can not be changed after they have been assigned once")
	}

	// Assign the streams the user specified and close them if we finish
	r.appendStreamWriterOnStartup(StdoutOut, streams.StdOUT)
	r.appendStreamWriterOnStartup(StderrOut, streams.StdERR)
	return r
}

// AttachStream Attach an arbitary writer to the given outputType, if you want to
// remove it use DetachStream(writer) to do so. Make sure to perform all closing
// operations yourself! Warning: Attaching a stream dynamically might fail in the
// case of a blocking Write taking longer than expected!
func (r *StdReader) AttachStream(outputType OutputType, writer io.Writer, timeout time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// No timeout is only permitted if the reader has not been set-up yet.
	if timeout <= 0 && r.invoked {
		log.Panic("timeout of <=0 provided to AttachStream after writer started capturing this might deadlock!", zap.Duration("timeout", timeout))
		return false
	}

	// Check if the stream already exists
	if _, ok := r.streamMap[writer]; ok {
		if ok {
			log.Error("writer already existed in the map, not adding again")
			return false
		}
	}

	// Check if the stream was appended correctly
	wasAdded := r.appendByOutputType(outputType, writer, timeout)
	if !wasAdded {
		return false
	}

	r.streamMap[writer] = true
	return true
}

// SetTerminateMainOnly Only request the main pid of the process to terminate
// This is dangerous and might leave processes behind, only use when you know
// what you are doing
func (r *StdReader) SetTerminateMainOnly() *StdReader {
	// Not permitted
	if r.invoked {
		log.Error("preventing termination mode change, already running")
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.useProcessGroup = false
	return r
}

// SetTerminationSignal use a custom graceful termination signal, some processes
// might need it to exit cleanly
func (r *StdReader) SetTerminationSignal(sig syscall.Signal) *StdReader {
	// Not permitted
	if r.invoked {
		log.Error("preventing termination signal change, already running")
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.terminationSignal = sig
	return r
}

// SetGracePeriod sets the amount of time that has to pass before the process is
// killed if it did not respond to the termination signal.
func (r *StdReader) SetGracePeriod(period time.Duration) *StdReader {
	// Not permitted
	if r.invoked {
		log.Error("preventing grace period change, already running")
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.gracePeriod = period
	return r
}

// SetFileWriteBufferSize sets the write buffer size for the files specified
func (r *StdReader) SetFileWriteBufferSize(size int) *StdReader {
	// Not permitted
	if r.invoked {
		log.Error("preventing file write buffer size change, already running")
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if size < 1 {
		log.Panic("file write buffer too small", zap.Int("requested", size))
		return nil
	}

	r.fileWriteBufferSize = size
	return r
}

// AlwaysKeepFiles if set to true files will be always kept, even if the process terminates early or without output
func (r *StdReader) AlwaysKeepFiles(val bool) *StdReader {
	// Not permitted
	if r.invoked {
		log.Error("preventing file retention change, already running")
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.alwaysKeepFiles = val
	return r
}

// Creates a file at the specified path
// Do not forget to call close on this!
func createFile(file *CaptureFile) (*os.File, error) {
	if _, err := os.Stat(file.path); os.IsNotExist(err) {
		dirPath, err := filepath.Abs(filepath.Dir(file.path))
		if err != nil {
			log.Error("failed to get absolute path", zap.String("path", dirPath))
			return nil, err
		}

		if err = os.MkdirAll(dirPath, file.dirperm); err != nil {
			log.Error("could not create required directories", zap.String("path", dirPath))
			return nil, err
		}
	}

	// Create the output file with restrictive permission
	outfile, err := os.OpenFile(file.path, file.flags, file.perm)
	if err != nil {
		log.Error("could not create output file", zap.String("file", file.path))
		return nil, err
	}

	return outfile, nil
}

type OutputType byte

const (
	StdoutOut OutputType = iota
	StderrOut
)

// This directly appends a writer for the given type, no closing is performed
func (r *StdReader) appendByOutputType(writerType OutputType, writer io.Writer, timeout time.Duration) bool {
	var targetWriter *DynamicMultiWriter
	switch writerType {
	case StdoutOut:
		targetWriter = r.stdOutMultiWriter
	case StderrOut:
		targetWriter = r.stdErrMultiWriter
	}

	// If no timeout was given this might be the following
	// 1) Multiwriters not created yet (!invoked)
	// 2) Internal call with timeout 0, guaranteed to not dead-lock
	if !r.invoked || timeout == 0 {
		targetWriter.Append(writer)
		return true
	}

	// Dynamic attaches might deadlock under certain conditions, specify timeout
	return targetWriter.RequestAppend(writer, timeout)
}

// Append the writer only if its not nil and return a closer if its closeable, this is guaranteed to work
func (r *StdReader) appendStreamWriterOnStartup(writerType OutputType, writer io.Writer) {
	if writer == nil {
		return
	}

	// Append the writer to the writer list without timeout
	if r.appendByOutputType(writerType, writer, 0) {
		r.streamMap[writer] = true
	}
}

func (r *StdReader) appendCaptureFileWriterIfSet(writerType OutputType, file *CaptureFile) (CloseFunc, error) {
	// Optional
	if file == nil {
		return EmptyCloseFunc, nil
	}

	// open the file for writing
	outfile, err := createFile(file)
	if err != nil {
		return EmptyCloseFunc, err
	}

	// Prepare and assign target stream
	concurrentSafeWriter := ConcurrentBufioWriter{
		sync.Mutex{},
		bufio.NewWriterSize(outfile, r.fileWriteBufferSize),
	}

	// Append the buffered writer to the writer list, without a timeout
	r.appendByOutputType(writerType, &concurrentSafeWriter, 0)

	// This tear-down function takes care of flushing, deleting (if empty & requested) and closing the file
	return func(cmdExitError *error) error {
		concurrentSafeWriter.Flush()

		fileName := outfile.Name()
		// User wants to keep files, skip deletion logic
		if r.alwaysKeepFiles {
			return outfile.Close()
		}

		// Get the file details
		stat, err := outfile.Stat()

		// If the user did not want to keep files at all cost, check if
		// 1) it is 0 bytes in size or
		// 2) Our process was not started correctly
		if (err != nil && stat.Size() == 0) ||
			// If a startup error occured, delete the file
			(cmdExitError != nil && errors.Is(*cmdExitError, &ProcessNotStartedError{})) {
			// Close the file before deletion
			_ = outfile.Close()
			log.Info("deleting output file", zap.String("file", fileName))
			return os.Remove(fileName)
		}

		return outfile.Close()
	}, nil
}

func (r *StdReader) capture() (err error) {
	r.mu.Lock()

	log.Debug("preparing command execution", zap.String("cmd", r.cmd.String()))

	// Default outputs to /dev/null
	r.cmd.Stdout = nil
	r.cmd.Stderr = nil

	// Even if its only one writer, we can use MultiWriter here
	if size := r.stdOutMultiWriter.Size(); size > 0 {
		r.cmd.Stdout = r.stdOutMultiWriter
		log.Info("writing stdout to multiple writers", zap.Int("count", size))
	}

	if size := r.stdErrMultiWriter.Size(); size > 0 {
		r.cmd.Stderr = r.stdErrMultiWriter
		log.Debug("writing stderr to multiple writers", zap.Int("count", size))
	}

	// Sanity check that we dont over-use this function
	if r.cmd.Stdout == nil && r.cmd.Stderr == nil {
		log.Error("std[out|err] skipped skipped on helper that is designed to handle their output, api misuse?")
	}

	// This requests a process group from the system, all spawned children will belong to it
	if r.useProcessGroup {
		log.Info("using a process group for the command")
		r.cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	// Unlock the mutex, nothing depends on those values now
	r.mu.Unlock()

	// Make sure we close everything and return the proper error at the end
	defer func(err *error) {
		r.closeEverything(err)

		// err can never be nil, we get it from the parent
		r.processExitChannel <- *err
	}(&err)

	// Start the cmd
	err = r.cmd.Start()
	log.Info("starting cmd", zap.String("cmd", r.cmd.String()))
	if err != nil {
		log.Error("could not start cmd", zap.Error(err))
		err = &ProcessNotStartedError{err.Error()}
		return
	}

	// Start termination handler, blocking
	err = r.GracefulTermination(r.cmd)

	// we need named parameter return to capture the err for the file closers
	return
}

// This forcefully detaches a writer from our reader list
func (r *StdReader) detachStreamInternal(writer io.Writer, bulkRemove bool) (wasClosed bool) {
	// Check if the requested writer exist
	if _, ok := r.streamMap[writer]; !ok {
		log.Error("writer not found, might have been closed already?")
		return false
	}

	// Close the stream first, otherwise this remove might dead-lock if the Write is stuck
	wasClosed = CloseIfCloseable(writer) == nil

	// Delete the entry from the stream map
	delete(r.streamMap, writer)

	// Return early if this is a bulk remove, if others streams are stuck we might hang
	if bulkRemove {
		return
	}

	// Try to remove the dynamically attached writer
	if !r.stdOutMultiWriter.Remove(writer) && !r.stdErrMultiWriter.Remove(writer) {
		log.Error("cant remove writer from multiwriters")
		return false
	}

	// Return if the close result was nil
	return
}

// Start starts the async capture
func (r *StdReader) Start() {
	r.mu.RLock()
	// Sanity check if the user already invoked us by accident
	if r.invoked {
		log.Panic("already running, undefined behavior, abort")
		return
	}
	// Already mark us as started
	r.invoked = true
	r.mu.RUnlock()

	go func() {
		// Ignore the capture result, someone needs to call wait anyway, or its not important
		_ = r.capture()
	}()
}

// Wait Might block forever if run was not called
func (r *StdReader) Wait() chan error {
	return r.processExitChannel
}

// Run Sync
func (r *StdReader) Run() error {
	r.Start()
	return <-r.Wait()
}

// DetachStream detaches an active writer
func (r *StdReader) DetachStream(writer io.Writer) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.detachStreamInternal(writer, false)
}
