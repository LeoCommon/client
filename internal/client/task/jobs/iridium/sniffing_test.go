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
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/streamhelpers"
	"disco.cs.uni-kl.de/apogee/pkg/test"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"go.uber.org/zap"
)

var (
	SCRIPT_DIR string = test.GetScriptPath("iridium")
	TMP_DIR    string
	TEST_START time.Time
	APP        *client.App
	JOB_NAME   string
)

func GetURL(path string) string {
	return api.GetBaseURL() + path + APP.Config.Client.Authentication.SensorName
}

func GetDataURL(job_name string) string {
	return api.GetBaseURL() + "/data/" + APP.Config.Client.Authentication.SensorName + "/" + job_name
}

func SetupMockAPI(t *testing.T) func() {
	// Set up fake urls
	APP.Config.Client.Provisioning.Host = "discosat-mock.lan"
	APP.Config.Client.Authentication.SensorName = "test_sensor"

	// Try setting up now
	client.SetupAPI(APP)

	mock := httpmock.NewMockTransport()

	mock.RegisterResponder("PUT", GetURL("/sensors/update/"), func(req *http.Request) (*http.Response, error) {
		return httpmock.NewStringResponse(200, ""), nil
	})

	// Register ZIP Uploader
	mock.RegisterResponder("POST", GetDataURL(JOB_NAME), func(req *http.Request) (*http.Response, error) {
		reader, err := req.MultipartReader()
		if err != nil {
			return nil, err
		}

		// Expected files
		iridiumFiles := []string{
			JOB_NAME + "_job.txt",
			JOB_NAME + "_startStatus.txt",
			"hackrf.conf",
			"output.bits",
			"output.stderr",
			JOB_NAME + "_endStatus.txt",
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

	httpmock.ActivateNonDefault(api.GetClient().GetClient())

	api.SetTransport(mock)

	return func() {
		// teardown
	}
}

func SetupIridiumTest(t *testing.T) func() {
	t.Helper()
	log.Init(true)
	TMP_DIR = t.TempDir()

	// Change to the scripts directory
	os.Chdir(SCRIPT_DIR)
	os.Setenv("PATH", os.Getenv("PATH")+":"+SCRIPT_DIR)

	// Prepare the client
	var err error
	APP, err = client.Setup(true)
	assert.NoError(t, err)

	// Set required config settings
	APP.Config.Client.Jobs.StoragePath = TMP_DIR + "/jobs/"
	APP.Config.Client.Jobs.TempPath = TMP_DIR

	// is there any benefit to making this random?
	JOB_NAME = "TEST_JOB"

	// Fake the api here
	SetupMockAPI(t)

	TEST_START = time.Now()

	// shared tear down logic, if any
	return func() {
		APP.CliFlags = nil
		APP.Shutdown()
		APP = nil
		goleak.VerifyNone(t)
	}
}

// This test passes the startup check but terminates afterwards
func TestSniffingProcessExitsBeforeEnd(t *testing.T) {
	defer SetupIridiumTest(t)()

	err := IridiumSniffing(api.FixedJob{
		Id:        "mock_test",
		Name:      JOB_NAME,
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + 10,
	}, APP)

	assert.NoError(t, err)
}

func TestIridiumSniffing(t *testing.T) {
	// Change to the realtime directory
	SCRIPT_DIR += "realtime/"
	defer SetupIridiumTest(t)()

	tests := []struct {
		name    string
		endTime int64
		wantErr error
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
				Name:      JOB_NAME,
				StartTime: time.Now().Unix(),
				EndTime:   tt.endTime,
			}, APP)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
