package iridium

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
	"disco.cs.uni-kl.de/apogee/internal/client/task/jobs"
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
	return App.Api.GetBaseURL() + path + App.Config.Api.SensorName
}

func GetDataURL(job_name string) string {
	return App.Api.GetBaseURL() + "/data/" + App.Config.Api.SensorName + "/" + job_name
}

func SetupMockAPI(t *testing.T) func() {
	t.Helper()

	// Set up fake urls
	App.Config.Api.Url = "discosat-mock.lan"
	App.Config.Api.SensorName = "test_sensor"

	// Try setting up the api now
	var err error
	App.Api, err = api.NewRestAPI(App.Config.Api)
	assert.NoError(t, err)

	mock := httpmock.NewMockTransport()

	mock.RegisterResponder("PUT", GetURL("/sensors/update/"), func(req *http.Request) (*http.Response, error) {
		return httpmock.NewStringResponse(200, ""), nil
	})

	// Register ZIP Uploader
	mock.RegisterResponder("POST", GetDataURL(JobName), func(req *http.Request) (*http.Response, error) {
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

	httpmock.ActivateNonDefault(App.Api.GetClient().GetClient())

	App.Api.SetTransport(mock)

	return func() {
		// teardown
	}
}

func SetupIridiumTest(t *testing.T) func() {
	t.Helper()
	log.Init(true)
	TmpDir = t.TempDir()

	// Change to the scripts directory
	os.Chdir(ScriptDir)
	os.Setenv("PATH", os.Getenv("PATH")+":"+ScriptDir)

	// Prepare the client
	var err error
	App, err = client.Setup(true)
	assert.NoError(t, err)

	// Set required config settings
	App.Config.Jobs.StoragePath = TmpDir + "/jobs/"
	App.Config.Jobs.TempPath = TmpDir

	// is there any benefit to making this random?
	JobName = "TEST_JOB"

	// Fake the api here
	SetupMockAPI(t)

	// shared tear down logic, if any
	return func() {
		App.CliFlags = nil
		App.Shutdown()
		App = nil
		goleak.VerifyNone(t)
	}
}

// This test passes the startup check but terminates afterwards
func TestSniffingProcessExitsBeforeEnd(t *testing.T) {
	defer SetupIridiumTest(t)()

	err := IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      JobName,
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + 10,
	}, App)

	assert.NoError(t, err)
}

func TestSniffingDisabled(t *testing.T) {
	defer SetupIridiumTest(t)()

	App.Config.Jobs.Iridium.Disabled = true

	err := IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      JobName,
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + 10,
	}, App)

	assert.ErrorIs(t, err, &jobs.DisabledError{})
}

func TestIridiumSniffing(t *testing.T) {
	// Change to the realtime directory
	ScriptDir += "realtime/"
	defer SetupIridiumTest(t)()

	tests := []struct {
		wantErr error
		name    string
		endTime int64
	}{
		{
			name:    "sniffing for 2 seconds",
			endTime: time.Now().Unix() + 2,
			wantErr: nil,
		},
		{
			name:    "terminated early",
			endTime: time.Now().Unix() - 2,
			wantErr: &streamhelpers.TerminatedEarlyError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IridiumSniffing(api.FixedJob{
				Id:        "mock_test",
				Name:      JobName,
				StartTime: time.Now().Unix(),
				EndTime:   tt.endTime,
			}, App)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
