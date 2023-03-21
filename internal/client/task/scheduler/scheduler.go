package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ExclusiveResource string
type ExclusiveResources []ExclusiveResource

type ExclusiveResourceMap map[ExclusiveResource]bool

const (
	SDRDevice1 ExclusiveResource = "SDRDevice1"
	FullCPU    ExclusiveResource = "FullCPU"
	// Add other exclusive resources here

	// The amount of time the scheduler sleeps between ticks
	// This allows 10 "concurrent" events per second with 1 worker, this is okay for now
	TickRate = 100 * time.Millisecond

	// Maximum job duration
	MaxTaskDuration = 24 * time.Hour
)

type JobFunction func(context.Context, interface{}) error

var (
	ErrTaskAlreadyRunning         = errors.New("an identical task is being executed already")
	ErrTaskAlreadyExists          = errors.New("an identical task already existed")
	ErrResourceSharingNotPossible = errors.New("cant share priority resources with overlapping task")
	ErrRunningTaskCantBeModified  = errors.New("an already running task can not be modified")
	ErrTaskNotFound               = errors.New("task not found")

	// Task validation errors
	ErrTaskIDInvalid           = errors.New("invalid/empty task id")
	ErrTaskInvalidHandler      = errors.New("task has no valid handler")
	ErrTaskTimesInvalid        = errors.New("specified times were invalid")
	ErrTaskMaxDurationExceeded = errors.New("task duration is too long")
)

type Task struct {
	StartTime          time.Time
	EndTime            time.Time
	Argument           interface{}
	Command            JobFunction
	cancelFunc         context.CancelFunc
	cancelOnce         sync.Once
	PreExecute         func() bool
	PostExecute        func(error)
	exclusiveResources ExclusiveResourceMap
	id                 string // An unique ID
}

// Cancel cancels a task (only once)
func (t *Task) Cancel() {
	t.cancelOnce.Do(func() {
		t.cancelFunc()
	})
}

// Equals checks if the user-supplied task parameters are the same
func (t *Task) Equals(other *Task) bool {
	if t == nil && other == nil {
		return true
	}
	if t == nil || other == nil {
		return false
	}
	return t.StartTime.Equal(other.StartTime) &&
		t.EndTime.Equal(other.EndTime) &&
		t.id == other.id
}

func (t *Task) HasResourceOverlap(other *Task) bool {
	// Check if an overlapping task uses the same resources as another task
	if (t.StartTime.Before(other.EndTime) || t.StartTime.Equal(other.EndTime)) &&
		(t.EndTime.After(other.StartTime) || t.EndTime.Equal(other.StartTime)) {

		// Check if any resource is also used by the overlapping task
		for k := range other.exclusiveResources {
			if _, ok := t.exclusiveResources[k]; ok {
				log.Debug("task cant share this exclusive resource", zap.String("resource", string(k)))
				return true
			}
		}
	}

	return false
}

func NewTask(startTime time.Time, endTime time.Time, command func(context.Context, interface{}) error, arg interface{}) *Task {
	return &Task{
		id:                 uuid.NewString(),
		StartTime:          startTime,
		EndTime:            endTime,
		Command:            command,
		Argument:           arg,
		cancelOnce:         sync.Once{},
		exclusiveResources: make(ExclusiveResourceMap),
	}
}

func (t *Task) WithResource(resources ...ExclusiveResource) *Task {
	for _, resource := range resources {
		t.exclusiveResources[resource] = true
	}
	return t
}

func (t *Task) WithID(id string) *Task {
	if len(id) == 0 {
		log.Panic("empty task id in scheduler will break it, panic")
	}

	t.id = id
	return t
}

type taskQueue []*Task

func (q taskQueue) Len() int { return len(q) }

func (q taskQueue) Less(i, j int) bool {
	return q[i].StartTime.Before(q[j].StartTime)
}

func (q taskQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *taskQueue) Push(x interface{}) {
	item := x.(*Task)
	*q = append(*q, item)
}

func (q *taskQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[0 : n-1]
	return item
}

type Scheduler struct {
	m       sync.RWMutex
	wg      sync.WaitGroup
	quit    chan struct{}
	workers chan struct{}
	// Queued tasks
	queue taskQueue
	// Running tasks
	running []*Task
}

func NewScheduler(numWorkers int) *Scheduler {
	return &Scheduler{
		queue:   make(taskQueue, 0),
		workers: make(chan struct{}, numWorkers),
		quit:    make(chan struct{}),
	}
}

func IsValidTask(task *Task) error {
	// If no ID was specified, the user might have altered it
	if len(task.id) == 0 {
		return ErrTaskIDInvalid
	}

	// The task needs a valid (!= nil) handler
	if task.Command == nil {
		return ErrTaskInvalidHandler
	}

	// If the start time is not <= end time dont schedule it
	if task.StartTime.After(task.EndTime) {
		return ErrTaskTimesInvalid
	}

	// Check if the job exceeds the maximum job duration
	if task.EndTime.Sub(task.StartTime) > MaxTaskDuration {
		return ErrTaskMaxDurationExceeded
	}

	return nil
}

// Update modifies a queued task with the same id
func (s *Scheduler) Update(newTask *Task) error {
	// Check if the task is valid
	if err := IsValidTask(newTask); err != nil {
		return err
	}

	s.m.Lock()
	defer s.m.Unlock()

	// Only touch queued tasks
	for i, task := range s.queue {
		if task.id == newTask.id {
			return s.modifyTaskAtIndex(i, newTask)
		}
	}

	return ErrTaskNotFound
}

// modifyTaskAtIndex modifies a given task at index idx
func (s *Scheduler) modifyTaskAtIndex(idx int, newTask *Task) error {
	// We have to go through the entire queue again to make sure we dont resource share with the new times
	if s.matchQueueEntry(func(t *Task) bool {
		// Skip the "old" entry
		if t.id == newTask.id {
			return false
		}

		return newTask.HasResourceOverlap(t)
	}) {
		// We found an overlap, so we cant modify the task and we should not keep the old one
		s.removeTaskFromQueue(idx)
		log.Warn("resource overlap, modification impossible, discarded orphaned task")
		return ErrResourceSharingNotPossible
	}

	// Everything fine, its safe to adjust the queued task
	s.heapFixInternal(idx, newTask)
	log.Info("Modified existing scheduled task")
	return nil
}

func (s *Scheduler) heapFixInternal(idx int, newTask *Task) {
	s.queue[idx] = newTask
	heap.Fix(&s.queue, idx)
}

// matchQueueEntry runs a match function on the queue and returns the match result
func (s *Scheduler) matchQueueEntry(matcher func(*Task) bool) bool {
	for i := 0; i < len(s.queue); i++ {
		if matcher(s.queue[i]) {
			return true
		}
	}

	return false
}

// Schedule schedules a task
func (s *Scheduler) Schedule(newTask *Task) error {
	// Check if the task is valid
	if err := IsValidTask(newTask); err != nil {
		return err
	}

	s.m.Lock()
	defer s.m.Unlock()

	// Check running tasks
	// 1) if we get a schedule call without changes for a running job we return a "harmless" ErrTaskAlreadyRunning error
	// 2) else we return the ErrRunningTaskCantBeModified error
	// 3) if there is a resource conflict with a running task
	for _, t := range s.running {
		// If the entire task matches this is condition 1)
		if t.Equals(newTask) {
			log.Debug("identical running task found, not doing anything")
			return ErrTaskAlreadyRunning
		}

		// If we only find an identical id, we are not allowed to modify it
		if newTask.id == t.id {
			log.Debug("no changes to running tasks allowed")
			return ErrRunningTaskCantBeModified
		}

		// make sure there is no resource overlap with a currently running task
		if newTask.HasResourceOverlap(t) {
			log.Debug("resource overlap with running task")
			return ErrResourceSharingNotPossible
		}
	}

	// At this point we know that the task is either
	// - completely new
	// - a modification of a queued task that does not use overlapping resources with any currently running tasks

	// Now check the queued tasks for
	// 1) completely identical tasks
	// 2) tasks with matching ids
	// 3) overlapping queued tasks using restricted resource
	for i := 0; i < len(s.queue); i++ {
		// Grab existing task from the queue
		eTask := s.queue[i]

		// If the exact same task already exists, we dont need to do anything
		if eTask.Equals(newTask) {
			log.Debug("task already existed, not doing anything")
			return ErrTaskAlreadyExists
		}

		// If the task was no full duplicate but the id is identical, the schedule changed
		// Modification is guaranteed to succeed as no other tasks are executed as long as we hold the lock
		if eTask.id == newTask.id {
			return s.modifyTaskAtIndex(i, newTask)
		}

		// Check if the new task uses the same resources as another task
		if result := newTask.HasResourceOverlap(eTask); result {
			return ErrResourceSharingNotPossible
		}
	}

	// We added a completely new task
	log.Debug("scheduled as completely new task")
	heap.Push(&s.queue, newTask)
	return nil
}

func (s *Scheduler) Cancel(id string) bool {
	s.m.Lock()
	defer s.m.Unlock()

	// Try to remove it from the scheduled list
	for i := 0; i < len(s.queue); i++ {
		if s.queue[i].id == id {
			s.removeTaskFromQueue(i)
			break
		}
	}

	// Try in the running task list
	return s.finishUpTask(id)
}

func (s *Scheduler) removeTaskFromQueue(idx int) {
	// Cancel the context
	s.queue[idx].Cancel()
	heap.Remove(&s.queue, idx)
}

// finishUpTask is an internal function that handles task completion
func (s *Scheduler) finishUpTask(id string) bool {
	// Try cancelling running task
	for i, t := range s.running {
		// Find the right task
		if t.id != id {
			continue
		}

		// Cancel the context
		t.Cancel()

		// remove the task from the running list
		s.running = append(s.running[:i], s.running[i+1:]...)
		return true
	}

	return false
}

func (s *Scheduler) Run() {
	// Create the main scheduler ticker
	ticker := time.NewTicker(TickRate)
	defer ticker.Stop()

tickLoop:
	for {
		select {
		case <-ticker.C:
			s.tick()

		case <-s.quit:
			break tickLoop
		}
	}

	log.Debug("scheduler run completed")
}

// tick performs a single scheduler tick
func (s *Scheduler) tick() {
	// Block modificatiosn for the entire duration of the tick
	s.m.Lock()
	defer s.m.Unlock()

	for len(s.queue) > 0 {
		// Grab the very next task from the list
		task := s.queue[0]

		// First element not ready yet
		if task.StartTime.After(time.Now().UTC()) {
			break
		}

		// No workers available
		if len(s.workers) == cap(s.workers) {
			break
		}
		s.workers <- struct{}{}
		heap.Pop(&s.queue)

		// If the duration is bigger than 0 create the context with a timeout
		var ctx context.Context
		if taskDuration := task.EndTime.Sub(time.Now().UTC()); taskDuration > 0 {
			ctx, task.cancelFunc = context.WithTimeout(context.Background(), taskDuration)
		} else {
			ctx, task.cancelFunc = context.WithCancel(context.Background())
		}

		// Add the task to the running list
		s.running = append(s.running, task)
		s.wg.Add(1)

		// Spawn the worker
		go func(ctx context.Context) {
			defer func() {
				// Always finish up the task, regardless of how it ended
				s.m.Lock()
				s.finishUpTask(task.id)
				s.m.Unlock()

				<-s.workers
				s.wg.Done()
			}()

			// If there is a pre-execute hook
			if task.PreExecute != nil {
				if !task.PreExecute() {
					log.Warn("task pre-execute function aborted the run")
					return
				}
			}

			// Run the task
			err := task.Command(ctx, task.Argument)
			if task.PostExecute != nil {
				task.PostExecute(err)
			} else if err != nil {
				log.Error("task finished with error", zap.Error(err))
			}

		}(ctx)
	}
}

// HasRunningJob returns true if at least one job is running, false otherwise
func (s *Scheduler) HasRunningJob() bool {
	s.m.RLock()
	defer s.m.RUnlock()

	return len(s.running) != 0
}

func (s *Scheduler) Shutdown() {
	// Set the shutdown flag
	s.quit <- struct{}{}

	// Cancel all running tasks
	s.m.RLock()
	for _, task := range s.running {
		task.Cancel()
	}
	s.m.RUnlock()

	// Wait for all jobs to terminate
	s.wg.Wait()

	// Clean the fields
	s.running = nil
	s.queue = nil
	s.workers = nil
}
