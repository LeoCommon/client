package main

import (
	"time"

	"disco.cs.uni-kl.de/apogee/internal/modem_manager/modem/sim7600"
	"disco.cs.uni-kl.de/apogee/internal/modem_manager/modem/sim7600/atparser"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"go.uber.org/zap"
)

const (
	DEBUG                   = true
	MODEM_START_RETRY_COUNT = 5    // Try 5 times
	MODEM_START_RETRY_WAIT  = 5000 // Wait time in milliseconds => 5 seconds between each tries
)

func main() {
	// Initialize logger
	log.Init(DEBUG)

	SIM7600Modem := sim7600.Create(nil)

	var err error

	for attempts := 0; attempts < MODEM_START_RETRY_COUNT; attempts++ {
		if attempts > 0 {
			time.Sleep(time.Duration(MODEM_START_RETRY_WAIT) * time.Millisecond)
		}

		err = SIM7600Modem.Open()
		if err != nil {
			log.Error("Failed to open modem interface", zap.Error(err))
			continue
		}

		err = SIM7600Modem.StartGPS(atparser.GpsModeStandalone, false)
		if err != nil {
			log.Error("Failed to start GPS on modem", zap.Error(err))
			continue
		}

		// Break out of the loop
		break
	}
}
