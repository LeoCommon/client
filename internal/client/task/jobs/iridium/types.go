package iridium

import (
	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api"
)

type SniffingConfig struct {
	CenterfrequencyKhz float64
	BandwidthKhz       float64
	Gain               int64
	BbGain             int64
	IfGain             int64
}

type SniffingJob struct {
	app            *client.App
	configFilePath string
	job            api.FixedJob
	// output file list
	outputFiles []string
	config      SniffingConfig
}

type StatusType string

const (
	StatusTypeStart StatusType = "startStatus"
	StatusTypeStop  StatusType = "endStatus"
)

type StartupResult struct {
	Err error
	Str string
}
