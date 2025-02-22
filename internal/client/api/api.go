package api

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	h "github.com/LeoCommon/client/internal/client/api/helpers"
	"github.com/LeoCommon/client/internal/client/api/jwt"
	"github.com/LeoCommon/client/internal/client/config"
	"github.com/LeoCommon/client/pkg/log"

	"github.com/imroc/req/v3"
)

type RestAPI struct {
	client *req.Client

	jwt *jwt.JwtHandler

	// Store these for later usage
	conf     *config.Manager
	cm       *config.ApiConfigManager
	clientCM *config.ClientConfigManager
}

func NewRestAPI(conf *config.Manager, debug bool) (*RestAPI, error) {
	a := RestAPI{}
	a.conf = conf

	a.cm = conf.Api()
	a.clientCM = conf.Client()

	//set up the connection
	a.client = req.C()

	if debug {
		a.client.EnableDebugLog()
	}

	// Get a copy of the api config
	apiConf := a.cm.C()

	// Set up the api base-url
	a.client.SetBaseURL(apiConf.Url)

	// Set up the certificate and authentication
	rootCert := apiConf.RootCertificate
	if len(rootCert) > 0 {
		a.client.SetRootCertsFromFile(rootCert)
	}

	if apiConf.Auth.Bearer != nil {
		// Verify that the refresh token is valid
		if err := jwt.Validate(apiConf.Auth.Bearer.Refresh); err != nil {
			log.Error("refresh token validation failed", zap.NamedError("reason", err))
			return nil, fmt.Errorf("trying to use bearer authentication with invalid refresh token")
		}

		log.Info("using bearer authorization")

		// Set up the handler and its hooks
		var err error
		a.jwt, err = jwt.NewJWTHandler(a.cm, a.client)
		if err != nil {
			return nil, err
		}
	} else if apiConf.Auth.Basic != nil {
		username, password := apiConf.Auth.Basic.Credentials()
		log.Info("using basic auth mechanism", zap.String("username", username))
		a.client.SetCommonBasicAuth(username, password)
	} else {
		log.Warn("no/invalid api authentication scheme specified")
	}

	if apiConf.AllowInsecure {
		// Skip TLS verification upon request
		a.client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

		log.Warn("!WARNING WARNING WARNING! DISABLED TLS CERTIFICATE VERIFICATION! !WARNING WARNING WARNING!")
	}

	// Some connection configurations
	a.client.SetTimeout(RequestTimeout)
	a.client.SetCommonRetryCount(3)
	a.client.SetCommonRetryBackoffInterval(RequestRetryMinWaitTime, RequestRetryMaxWaitTime)

	return &a, nil
}

func (a *RestAPI) GetBaseURL() string {
	if a.client == nil {
		log.Panic("no client, cant get base url")
	}

	return a.client.BaseURL
}

// GetClient Use this for tests to set the transport to mock
func (a *RestAPI) GetClient() *req.Client {
	return a.client
}

func (r *RestAPI) PutSensorUpdate(status SensorStatus) error {
	resp, err := r.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(status).
		Put("sensors/update/" + r.clientCM.C().SensorName)

	return h.ErrorFromResponse(err, resp)
}

func (r *RestAPI) GetJobs() ([]FixedJob, error) {
	respCont := FixedJobResponse{}
	resp, err := r.client.R().
		SetHeader("Accept", "application/json").
		SetSuccessResult(&respCont).
		Get("fixedjobs/" + r.clientCM.C().SensorName)

	if err != nil {
		return []FixedJob{}, err
	}

	return respCont.Data, h.ErrorFromResponse(nil, resp)
}

func (r *RestAPI) PutJobUpdate(jobName string, status string) error {
	//TODO: change job_name to jobID
	if !(strings.HasPrefix(status, "running") ||
		strings.HasPrefix(status, "finished") ||
		strings.HasPrefix(status, "failed")) {
		return errors.New("status has to start with 'running', 'finished' or 'failed'")
	}
	resp, err := r.client.R().
		Put("fixedjobs/" + r.clientCM.C().SensorName + "?job_name=" + jobName + "&status=" + status)

	//resp, err := r.client.R().Put("fixedjobs/update/" + jobID + "?sensor_name=" + r.clientCM.C().SensorName + "&status=" + status)

	return h.ErrorFromResponse(err, resp)
}

//func (r *RestAPI) PostSensorData(ctx context.Context, jobName string, fileName string, filePath string) error {
//	// Upload the file
//	//TODO: implement chunk-uploading (current fix is bad: increase timeout)
//	r.client.SetTimeout(90 * time.Second)
//	resp, err := r.client.R().
//		// Set the context so we can abort
//		SetContext(ctx).
//		SetFile("in_file", filePath).
//		EnableForceChunkedEncoding().
//		SetUploadCallbackWithInterval(func(info req.UploadInfo) {
//			log.Info("intermediate upload progress", zap.String("file", info.FileName), zap.Float64("pct", float64(info.UploadedSize)/float64(info.FileSize)*100.0))
//		}, 1*time.Second).
//		Post("data/" + r.clientCM.C().SensorName + "/" + jobName)
//
//	r.client.SetTimeout(RequestTimeout) //TODO: implement chunk-uploading
//	// gather information for possible upload-timeout errors
//	log.Debug("end uploading the job-zip-file", zap.String("fileName", fileName))
//	return h.ErrorFromResponse(err, resp)
//}

func (r *RestAPI) PostChunk(ctx context.Context, sensorName string, jobID string, chunkFilePath string, chunkNr int, chunksRemaining int, chunkFileMD5 string) error {
	resp, err := r.client.R().
		SetContext(ctx). // Set the context so we can abort
		SetFile("in_file", chunkFilePath).
		EnableForceChunkedEncoding().
		SetUploadCallbackWithInterval(func(info req.UploadInfo) {
			log.Info("chunk "+fmt.Sprint(chunkNr)+"/"+fmt.Sprint(chunkNr+chunksRemaining)+" upload progress", zap.String("file", info.FileName), zap.Float64("pct", float64(info.UploadedSize)/float64(info.FileSize)*100.0))
		}, 1*time.Second).
		Post("data/upload/" + sensorName + "/" + jobID + "?chunk_nr=" + fmt.Sprint(chunkNr) + "&chunks_remaining=" + fmt.Sprint(chunksRemaining) + "&chunk_md5=" + chunkFileMD5)
	if err != nil {
		log.Error("error posting chunk", zap.String("file", chunkFilePath), zap.Error(err))
		return err
	} else if resp.StatusCode != 200 {
		log.Error("error in posting chunk response", zap.Int("code", resp.StatusCode), zap.String("status", resp.Status))
		return errors.New("UploadChunk.ResponseStatus = " + resp.Status)
	}
	if strings.Contains(resp.String(), "error") {
		log.Error("error in posting chunk response", zap.Int("code", resp.StatusCode), zap.String("status", resp.Status))
		return errors.New("UploadChunk.ResponseStatus = " + resp.Status)
	}
	return nil
}

//func (r *RestAPI) PostSensorData(ctx context.Context, jobID string, filePath string) error {
//	chunkSizeByte := r.conf.GetUploadChunkSize()
//	sensorName := r.conf.SensorName()
//	chunkFolder := filepath.Join(r.conf.JobTempPath(), jobID)
//	//split the file in chunks
//
//	chunkPaths, chunkMD5sums, err := file.SplitFileInChunks(filePath, filepath.Base(filePath), chunkFolder, chunkSizeByte)
//	if err != nil {
//		log.Error("error while creating chunks", zap.Error(err))
//		return err
//	}
//
//	// upload the single chunks
//	chunksAmount := len(chunkPaths)
//	for chunkNr, chunkPath := range chunkPaths {
//		chunkMD5 := chunkMD5sums[chunkNr]
//		chunkRemaining := chunksAmount - chunkNr - 1
//		err = r.PostChunk(ctx, sensorName, jobID, chunkPath, chunkNr, chunkRemaining, chunkMD5)
//		if err != nil {
//			log.Error("error while posting chunk", zap.Int("chunkNr", chunkNr), zap.Int("chunkRemaining", chunkRemaining), zap.Error(err))
//			return err
//		}
//	}
//
//	//delete the local chunks
//	err = file.DeleteChunks(chunkPaths)
//	if err != nil {
//		log.Error("error while deleting chunks", zap.Error(err))
//		return err
//	}
//
//	return nil
//}

func (r *RestAPI) PostSensorData(ctx context.Context, jobID string, filePath string) error {
	// create the single chunk and upload it directly (otherwise: out-of-memory-errors for very large files, with many chunks)
	// src: https://gist.github.com/serverwentdown/03d4a2ff23896193c9856da04bf36a94
	chunkSizeByte := r.conf.GetUploadChunkSize()
	sensorName := r.conf.SensorName()
	chunkFolder := filepath.Join(r.conf.JobTempPath(), jobID)
	fileBaseName := filepath.Base(filePath)
	//split the file in chunks

	// Open file to upload
	inFile, err := os.Open(filePath)
	defer func(inFile *os.File) {
		err := inFile.Close()
		if err != nil {
			log.Error("error while closing uploaded file after the upload", zap.String("uploadFile", filePath), zap.Error(err))
		}
	}(inFile)
	if err != nil {
		return err
	}

	// calculate how many chunks there will be
	fi, err := inFile.Stat()
	if err != nil {
		return err
	}
	fileSize := int(fi.Size())
	chunksAmount := int(math.Ceil(float64(fileSize) / float64(chunkSizeByte)))

	// ensure the output-folder is available
	err = os.MkdirAll(chunkFolder, os.ModePerm)
	if err != nil {
		log.Error("error while creating out-folder for chunk files", zap.Error(err))
		return err
	}

	for i := 0; i < chunksAmount; i++ {
		// create the chunk
		chunkPath := filepath.Join(chunkFolder, fileBaseName) + "_part" + fmt.Sprint(i)
		// Check for existing chunk (and delete if exists)
		if _, err = os.Stat(chunkPath); !os.IsNotExist(err) {
			err = os.Remove(chunkPath)
			if err != nil {
				log.Error("error while deleting pre-existing chunk-file", zap.String("chunkPath", chunkPath), zap.Error(err))
				return err
			}
		}

		// Create chunk file
		outFile, err := os.Create(chunkPath)
		if err != nil {
			log.Error("error while creating chunk file", zap.Error(err))
			return err
		}

		// Copy chunkSizeByte bytes to chunk file
		_, err = io.CopyN(outFile, inFile, int64(chunkSizeByte))
		if err == io.EOF {
			//fmt.Printf("%d bytes written to last file\n", written)
		} else if err != nil {
			log.Error("error while copying chunk file", zap.Error(err))
			return err
		}
		err = outFile.Close()
		if err != nil {
			log.Error("error while closing chunk file", zap.Error(err))
			return err
		}

		// Get the MD5 hash
		fileHash, err := os.Open(chunkPath)
		if err != nil {
			log.Error("error while opening chunk file for MD5 hash", zap.Error(err))
			return err
		}
		hash := md5.New()
		if _, err := io.Copy(hash, fileHash); err != nil {
			log.Error("error while creating MD5 hash", zap.Error(err))
			return err
		}
		hashString := hex.EncodeToString(hash.Sum(nil))
		err = fileHash.Close()
		if err != nil {
			log.Error("error while closing chunk file after MD5 hash", zap.Error(err))
			return err
		}

		// upload the chunk
		chunksRemaining := chunksAmount - i - 1
		err = r.PostChunk(ctx, sensorName, jobID, chunkPath, i, chunksRemaining, hashString)
		if err != nil {
			return err
		}

		// remove the chunk, before continuing with the next one
		err = os.Remove(chunkPath)
		if err != nil {
			log.Error("error while removing chunk file", zap.Error(err))
			return err
		}

	}

	return nil
}
