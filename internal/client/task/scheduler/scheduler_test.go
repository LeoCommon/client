package scheduler

import (
	"context"
	"testing"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/log"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestScheduler(t *testing.T) {
	log.Init(true)

	// Run with one worker, otherwise the order of the "same time" tasks is not deterministic
	s := NewScheduler(1)
	go s.Run()

	// Create a channel to communicate the execution of the tasks
	ch := make(chan string)

	task1 := NewTask(time.Now().Add(time.Second*2), time.Now().Add(time.Second*4), func(_ context.Context, _ interface{}) error {

		ch <- "task1 executed"
		return nil
	}, nil)

	// Runs at the "same" time as task1 but doesnt share resources
	task2 := NewTask(time.Now().Add(time.Second*2), time.Now().Add(time.Second*4), func(_ context.Context, _ interface{}) error {

		ch <- "task2 executed"
		return nil
	}, nil)

	task3 := NewTask(time.Now().Add(time.Second*4), time.Now().Add(time.Second*6), func(_ context.Context, _ interface{}) error {

		ch <- "task3 executed"
		return nil
	}, nil)

	assert.NoError(t, s.Schedule(task1))
	assert.NoError(t, s.Schedule(task2))
	assert.NoError(t, s.Schedule(task3))

	// Verify that task3 is executed at the correct time
	select {
	case msg := <-ch:
		if msg != "task1 executed" {
			t.Errorf("Expected task1 to execute, but got: %s", msg)
		}
	case <-time.After(time.Second * 5):
		t.Error("Timeout waiting for task1 to execute")
	}

	select {
	case msg := <-ch:
		if msg != "task2 executed" {
			t.Errorf("Expected task2 to execute, but got: %s", msg)
		}
	case <-time.After(time.Second * 1):
		t.Error("Timeout waiting for task2 to execute")
	}

	// Verify that task2 is executed at the correct time
	select {
	case msg := <-ch:
		if msg != "task3 executed" {
			t.Errorf("Expected task3 to execute, but got: %s", msg)
		}
	case <-time.After(time.Second * 3):
		t.Error("Timeout waiting for task3 to execute")
	}

	s.Shutdown()
}

func TestSchedulerSpam(t *testing.T) {
	log.Init(true)
	s := NewScheduler(4)
	go s.Run()

	for i := 0; i < 25; i++ {
		task1 := NewTask(time.Now(), time.Now().Add(time.Second*2), func(ctx context.Context, arg interface{}) error {
			log.Info("hello from task", zap.Int("i", arg.(int)))
			select {
			case <-ctx.Done():
				break
			case <-time.After(1 * time.Second):
				break
			}

			return nil
		}, i)

		assert.NoError(t, s.Schedule(task1))
	}

	time.Sleep(1 * time.Second)
	assert.True(t, s.HasRunningJob())

	// This does not wait for "not running" jobs
	s.Shutdown()
}

func TestSchedulerWithResources(t *testing.T) {
	log.Init(true)
	s := NewScheduler(2)

	// Create a task that uses the resource from 2 to 4 seconds after starting
	task1 := NewTask(
		time.Now().Add(time.Second*2),
		time.Now().Add(time.Second*4),
		func(ctx context.Context, arg interface{}) error {
			return nil
		},
		nil,
	).WithResource(SDRDevice1)

	// Create another task that also uses the resource from 3 to 5 seconds after starting
	task2 := NewTask(
		time.Now().Add(time.Second*3),
		time.Now().Add(time.Second*5),
		func(ctx context.Context, args interface{}) error {
			return nil
		},
		nil,
	).WithResource(SDRDevice1)

	// Create a task with identical times as task1
	task3 := NewTask(
		task1.StartTime,
		task1.EndTime,
		func(ctx context.Context, args interface{}) error {
			return nil
		},
		nil,
	).WithResource(SDRDevice1)

	// Create a task with non overlapping schedules
	task4 := NewTask(
		task1.EndTime.Add(1*time.Second),
		task1.EndTime.Add(4*time.Second),
		func(ctx context.Context, args interface{}) error {
			return nil
		},
		nil,
	).WithResource(SDRDevice1)

	// Schedule task 1
	s.Schedule(task1)

	// Should prevent adding the overlapping task with overlapping times
	err := s.Schedule(task2)
	assert.ErrorIs(t, err, ErrResourceSharingNotPossible)

	// Should prevent adding the overlapping task with identical times
	err = s.Schedule(task3)
	assert.ErrorIs(t, err, ErrResourceSharingNotPossible)

	// Should permit adding non-overlapping task
	err = s.Schedule(task4)
	assert.ErrorIs(t, err, nil)
}
