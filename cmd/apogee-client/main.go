package main

import (
	"fmt"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"disco.cs.uni-kl.de/apogee/pkg/task/handler"
	"go.uber.org/zap"
)

func main() {
	app, err := apogee.Setup()
	if err != nil {
		fmt.Printf("Initialization failed, error: %s\n", err)
		return // Exit
	}

	// Register the job handler
	handler, err := handler.NewJobHandler(&app)

	if err != nil {
		apglog.Fatal("Could not start job handler, aborting", zap.Error(err))
		return
	}

	// Run the application mainloop (blocking)

	// At this point the app struct is filled and we can use it
	clientConfig := app.Config.Client
	jobTicker := time.NewTicker(time.Duration(clientConfig.PollingInterval) * time.Second)
	app.WG.Add(1)

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed
	go func() {
		// Initial tick
		handler.Checkin()

		apglog.Info("task handler check-in completed, marking system as healthy, start polling")
		slot, err := app.OtaService.MarkBooted(rauc.SLOT_STATUS_GOOD)
		if err != nil {
			apglog.Error("OTA HealthCheck marking failed, continuing operation", zap.String("slot", slot), zap.Error(err))
		}

		for {
			select {
			case <-jobTicker.C:
				// Signal the jobhandler to tick
				handler.Tick()

			case <-app.ExitSignal:
				apglog.Info("exit signal received - shutting down tasks and routines")
				jobTicker.Stop()
				app.WG.Done()
				return
			}
		}
	}()

	// Wait until everything terminates
	app.WG.Wait()

	apglog.Info("pending tasks and routines terminated")

	// Shutdown everything
	app.Shutdown()

	// Stop the tasks
	handler.Shutdown()

	// Final greetings :)
	apglog.Info("stopped observing the sky!")
}
