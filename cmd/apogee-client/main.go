package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"disco.cs.uni-kl.de/apogee/pkg/task/handler"
	"go.uber.org/zap"
)

func TroubleShootConnectivity(app *apogee.App, err error) bool {
	if !app.NetworkService.HasConnectivity() {
		apglog.Error("We dont have network connectivity, try fall-back configurations")
		// #todo reconfigure network here
		return false
	}

	apglog.Debug("Network connectivity is looking fine, continuing")

	// Terminate the application, let systemd restart us
	// app.ExitSignal <- nil

	return true
}

func main() {
	app, err := apogee.Setup()
	if err != nil || app == nil {
		fmt.Printf("Initialization failed, error: %s\n", err)
		return // Exit
	}

	// Register the job handler
	handler, err := handler.NewJobHandler(app)

	if err != nil {
		apglog.Fatal("Could not start job handler, aborting", zap.Error(err))
		return
	}

	// At this point the app struct is filled, and we can use it
	clientConfig := app.Config.Client
	jobTicker := time.NewTicker(time.Duration(clientConfig.PollingInterval) * time.Second)
	app.WG.Add(1)

	EXIT_CODE := 0
	var rebootCMD *exec.Cmd = nil

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed.
	go func() {
		IsRebootPending := func(skipHandler bool) bool {
			// Check if the reboot marker exists and if we can safely reboot
			if rebootMarkerExists() {
				apglog.Info("Reboot marker detected")

				if skipHandler || !handler.HasRunningJob() {
					apglog.Info("Preparing soft-reboot", zap.Bool("SkipHandler", skipHandler))
					rebootCMD = cli.PrepareSoftReboot()
					return true
				}
			}

			return false
		}

		TerminateLoop := func() {
			jobTicker.Stop()
			app.WG.Done()
		}

		// Check if we have an imminent reboot this early
		if IsRebootPending(true) {
			apglog.Info("Skipping checkin, terminating early ...")
			TerminateLoop()
			return
		}

		// Initial tick
		err := handler.Checkin()

		// Check in failed
		if err != nil {
			apglog.Warn("initial check-in failed, running troubleshooter", zap.Error(err))

			// TroubleShooter failed, Terminate
			if !TroubleShootConnectivity(app, err) {
				// Critical fault, set EXIT_CODE = 1 and let systemd restart us
				EXIT_CODE = 1
				TerminateLoop()
				return
			}

			apglog.Warn("troubleshooter said service state is fine, continuing")
		}

		apglog.Info("task handler check-in completed, marking system as healthy, start polling")
		slot, err := app.OtaService.MarkBooted(rauc.SLOT_STATUS_GOOD)
		if err != nil {
			apglog.Error("OTA HealthCheck marking failed, continuing operation", zap.String("slot", slot), zap.Error(err))
		}

		for {
			select {
			case <-jobTicker.C:
				EntertainWatchdog()

				// This is just in case we missed a signal
				if IsRebootPending(false) {
					TerminateLoop()
					return
				}

				// Try to check-in with the server
				err := handler.Checkin()
				if err != nil {
					// Terminate if the troubleshooter found some problem
					if !TroubleShootConnectivity(app, err) {
						EXIT_CODE = 1
						TerminateLoop()
						return
					}

					continue
				}

				// Signal the job-handler to tick
				handler.Tick()

			case <-app.ReloadSignal:
				apglog.Info("reload signal received")

				EntertainWatchdog()

				if IsRebootPending(false) {
					TerminateLoop()
					return
				}

			case <-app.ExitSignal:
				apglog.Info("exit signal received - shutting down tasks and routines")

				EntertainWatchdog()
				IsRebootPending(true)
				TerminateLoop()
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

	// Perform a pending reboot if it is scheduled
	if rebootCMD != nil {
		if err = rebootCMD.Run(); err != nil {
			apglog.Error("Could not reboot, thats problematic ...", zap.Error(err))
		}
	}

	// Just making sure we exit with the proper code, so we dont get re-started by systemd
	os.Exit(EXIT_CODE)
}
