package iridium

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/config"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs/schema"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/streamhelpers"
	"disco.cs.uni-kl.de/apogee/pkg/test"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

var (
	ScriptDir string = test.GetScriptPath("iridium")
	TmpDir    string
	App       *client.App
	JobName   string
)

func GetURL(path string) string {
	return App.Api.GetBaseURL() + path + App.Conf.Client().C().SensorName
}

func GetDataURL(job_name string) string {
	return App.Api.GetBaseURL() + "/data/" + App.Conf.Client().C().SensorName + "/" + job_name
}

func SetupMockAPI(t *testing.T) func() {
	t.Helper()

	// Try setting up the api now
	var err error
	App.Api, err = api.NewRestAPI(App.Conf, true)
	assert.NoError(t, err)

	// Start mock
	httpmock.ActivateNonDefault(App.Api.GetClient().GetClient())

	httpmock.RegisterResponder("PUT", GetURL("/sensors/update/"), func(req *http.Request) (*http.Response, error) {
		return httpmock.NewStringResponse(200, ""), nil
	})

	// Register ZIP Uploader
	httpmock.RegisterResponder("POST", GetDataURL(JobName), func(req *http.Request) (*http.Response, error) {
		reader, err := req.MultipartReader()
		if err != nil {
			return nil, err
		}

		// Expected files
		iridiumFiles := []string{
			JobName + "_job.txt",
			JobName + "_startStatus.txt",
			"hackrf.conf",
			"output.bits",
			"output.stderr",
			JobName + "_endStatus.txt",
			"serviceLog.txt",
		}

		part, err := reader.NextPart()
		assert.NoError(t, err)
		assert.Equal(t, "in_file", part.FormName())
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, part)
		assert.NoError(t, err)
		r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		assert.NoError(t, err)

		// Check if all the files exist
		for _, f := range r.File {
			assert.Contains(t, iridiumFiles, f.Name)
			log.Debug("checked file", zap.String("file", f.Name))
		}

		return httpmock.NewStringResponse(200, ""), nil

	})

	return func() {
		// teardown
	}
}

func SetupIridiumTest(t *testing.T) func() {
	t.Helper()
	log.Init(true)
	TmpDir = t.TempDir()

	// Change to the scripts directory
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", oldPath+":"+ScriptDir)

	// Prepare the client
	var err error
	App, err = client.Setup(true)
	assert.NoError(t, err)

	// Set required config settings
	App.Conf.Job().Set(func(c *config.JobsConfig) {
		c.TempDir = config.TempPath(TmpDir)
		c.StorageDir = config.StoragePath(TmpDir + "/jobs/")
	})

	// is there any benefit to making this random?
	JobName = "TEST_JOB"

	// Fake the api here
	SetupMockAPI(t)

	// shared tear down logic, if any
	return func() {
		App.Shutdown()
		App = nil
		goleak.VerifyNone(t)
		os.Setenv("PATH", oldPath)
	}
}

// This test passes the startup check but terminates afterwards
func TestSniffingProcessExitsBeforeEnd(t *testing.T) {
	defer SetupIridiumTest(t)()

	err := IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      JobName,
		StartTime: time.Now().UTC(),
		EndTime:   time.Now().UTC().Add(10 * time.Second),
	}, context.Background(), &schema.JobParameters{App: App, Config: config.JobsConfig{}})

	assert.NoError(t, err)
}

func TestSniffingDisabled(t *testing.T) {
	defer SetupIridiumTest(t)()

	err := IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      JobName,
		StartTime: time.Now().UTC(),
		EndTime:   time.Now().UTC().Add(10 * time.Second),
	}, context.Background(), &schema.JobParameters{
		App: App,
		Config: config.JobsConfig{
			// Disable the iridium job
			Iridium: config.BaseJobSettings{Disabled: true},
		}})

	assert.ErrorIs(t, err, jobs.ErrJobDisabled)
}

func TestIridiumSniffingContextCanceled(t *testing.T) {
	defer SetupIridiumTest(t)()

	// Set up the test case
	tt := struct {
		endTime time.Time
		wantErr error
	}{
		endTime: time.Now().UTC().Add(1 * time.Second),
		wantErr: context.Canceled,
	}

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), tt.endTime.Sub(time.Now().UTC()))
	defer cancel()

	// Start the IridiumSniffing function in a separate goroutine
	done := make(chan error)
	go func() {
		done <- IridiumSniffing(api.FixedJob{
			Id:        "mock_test",
			Name:      JobName,
			StartTime: time.Now().UTC(),
			EndTime:   tt.endTime,
		}, ctx, &schema.JobParameters{App: App, Config: config.JobsConfig{}})
	}()

	// Wait a bit to make sure IridiumSniffing has started
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Check that the context is canceled
	assert.ErrorIs(t, ctx.Err(), context.Canceled)

	// Check that IridiumSniffing returns the expected error
	err := <-done
	assert.ErrorIs(t, err, tt.wantErr)
}

func TestIridiumSniffing(t *testing.T) {
	tests := []struct {
		wantErr  error
		name     string
		duration time.Duration
	}{
		{
			name:     "sniffing for 2 seconds",
			duration: 2 * time.Second,
			wantErr:  nil,
		},
		{
			name:     "terminated early",
			duration: 1 * time.Millisecond,
			wantErr:  &streamhelpers.TerminatedEarlyError{},
		},
	}

	// Change to the realtime directory
	ScriptDir += "realtime/"
	defer SetupIridiumTest(t)()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NOW := time.Now().UTC()
			ctx, cancel := context.WithTimeout(context.Background(), tt.duration)

			err := IridiumSniffing(api.FixedJob{
				Id:        "mock_test",
				Name:      JobName,
				StartTime: NOW,
				EndTime:   NOW.Add(tt.duration),
			}, ctx, &schema.JobParameters{App: App, Config: config.JobsConfig{}})

			assert.ErrorIs(t, err, tt.wantErr)

			// We invoke cancel for completeness sake
			cancel()

			// This is where the code would realize that we got terminated
			assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		})

	}
}
