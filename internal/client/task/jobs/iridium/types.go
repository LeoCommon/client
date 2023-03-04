package iridium

import (
	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
)

type SniffingConfig struct {
	CenterFrequency_khz float64
	Bandwidth_khz       float64
	Gain                int64
	Bb_gain             int64
	If_gain             int64
}

type IridiumSniffingJob struct {
	app *client.App
	job api.FixedJob

	config         SniffingConfig
	configFilePath string

	// output file list
	outputFiles []string
}

type StatusType string

const (
	StatusTypeStart StatusType = "startStatus"
	StatusTypeStop  StatusType = "endStatus"
)

type StartupResult struct {
	String string
	Error  error
}
