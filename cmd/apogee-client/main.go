package main

import (
	"fmt"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
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

	// At this point the app struct is filled, and we can use it
	clientConfig := app.Config.Client
	jobTicker := time.NewTicker(time.Duration(clientConfig.PollingInterval) * time.Second)
	app.WG.Add(1)

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed.
	go func() {
		// Counter & reboot-threshold for the failed intermediate checkins.
		checkinFails := 0
		checkinRebootTh := 3

		// Initial tick
		err := handler.Checkin()
		if err != nil {
			// If this doesn't work, try to figure out what is wrong or directly reboot, no retries.
			apglog.Error("Initial server checkin failed, reboot system", zap.Error(err))
			err = cli.RebootSystem()
			if err != nil {
				apglog.Error("Initial system reboot failed", zap.Error(err))
				apglog.Error("No server connection, reboot attempt failed ... try to continue and hope for the best.")
			}
		}

		apglog.Info("task handler check-in completed, marking system as healthy, start polling")
		slot, err := app.OtaService.MarkBooted(rauc.SLOT_STATUS_GOOD)
		if err != nil {
			apglog.Error("OTA HealthCheck marking failed, continuing operation", zap.String("slot", slot), zap.Error(err))
		}

		for {
			select {
			case <-jobTicker.C:
				// Signal the job-handler to tick
				time1 := time.Now().String()
				apglog.Debug("perform intermediate checkin " + time1)
				err := handler.Checkin()
				time2 := time.Now().String()
				apglog.Debug("performed intermediate checkin " + time2)
				if err != nil {
					apglog.Debug("checkin-error received:", zap.Error(err))
					checkinFails += 1
					if checkinFails >= checkinRebootTh {
						apglog.Error("Too many intermediate server checkins failed, reboot", zap.Int("checkinFails", checkinFails), zap.Error(err))
						time.Sleep(60 * time.Second) // pause, for debugging I have a chance during debugging making a screenshot
						err = cli.RebootSystem()
						if err != nil {
							apglog.Error("Intermediate system reboot failed", zap.Error(err))
						}
					} else {
						apglog.Error("Intermediate server checkin failed, retry later", zap.Int("checkinFails", checkinFails), zap.Error(err))
					}
				} else {
					// when no error appears, reset counter and continue pulling the jobs
					apglog.Debug("checkin worked fine, continue...")
					checkinFails = 0
					handler.Tick()
				}

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
