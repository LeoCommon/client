package main

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	"disco.cs.uni-kl.de/apogee/internal/client"
	"disco.cs.uni-kl.de/apogee/internal/client/api/helpers"
	jwt "disco.cs.uni-kl.de/apogee/internal/client/api/jwt/misc"
	"disco.cs.uni-kl.de/apogee/internal/client/constants"
	"disco.cs.uni-kl.de/apogee/internal/client/task/handler"
	"disco.cs.uni-kl.de/apogee/pkg/log"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"disco.cs.uni-kl.de/apogee/pkg/systemd"
	"go.uber.org/zap"
)

// verifyServiceHealth is designed to handle common faults with the service and decide if
// it returns false if client termination is required and true if the error is recoverable
func verifyServiceHealth(app *client.App, e error) bool {
	// Terminate the client if the refresh token is invalid
	if e == jwt.ErrRefreshTokenInvalid {
		return false
	}

	// Check if its an API related Error
	if urlErr, ok := e.(*url.Error); ok {
		// Grab the underlying error
		err := urlErr.Err

		// Check for "deal-breaker" errors
		if certerror, ok := err.(x509.UnknownAuthorityError); ok {
			log.Error("self signed certificate used without proper root_certificate entry, exiting", zap.Error(certerror))
			return false
		} else if certerror, ok := err.(x509.HostnameError); ok {
			log.Error("certificate hostname error, exiting", zap.Error(certerror))
			return false
		} else if certerror, ok := err.(x509.CertificateInvalidError); ok {
			log.Error("the encountered certificate was deemed invalid", zap.Error(certerror))
			return false
		}
	}

	// Check if its an api response error
	if respErr, ok := e.(*helpers.ResponseError); ok {
		switch respErr.Code {
		// This is the only "permanent" error
		case http.StatusUnauthorized:
			log.Error("api denied our authentication, unlikely to change, exiting", zap.Error(respErr))
			return false
		// Everything else will fix itself if we keep running
		default:
			log.Error("(temporary) server error encountered, continuing", zap.Error(respErr))
			return true
		}
	}

	// If we did not match anything so far, try to check network connectivity
	if !app.NetworkService.HasConnectivity() {
		log.Error("We dont have network connectivity, try fall-back configurations")
		// #todo reconfigure network here
		return false
	}

	log.Debug("service state is looking fine, continuing")
	return true
}

func rebootMarkerExists() bool {
	_, err := os.Stat(constants.RebootPendingTmpfile)
	return !os.IsNotExist(err)
}

func main() {
	app, err := client.Setup(false)
	if err != nil || app == nil {
		fmt.Printf("Initialization failed, error: %s\n", err)
		return // Exit
	}

	// Register the job handler
	handler, err := handler.NewJobHandler(app)

	if err != nil {
		log.Fatal("Could not start job handler, aborting", zap.Error(err))
		return
	}

	// At this point the app struct is filled, and we can use it
	jobTicker := time.NewTicker(time.Duration(app.Conf.Job().C().PollingInterval))
	app.WG.Add(1)

	EXIT_CODE := 0
	var rebootCMD *exec.Cmd = nil

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed.
	go func() {
		IsRebootPending := func(skipHandler bool) bool {
			// Check if the reboot marker exists and if we can safely reboot
			if rebootMarkerExists() {
				log.Info("Reboot marker detected")

				if skipHandler || !handler.HasRunningJob() {
					log.Info("Preparing soft-reboot", zap.Bool("SkipHandler", skipHandler))
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
			log.Info("Skipping checkin, terminating early ...")
			TerminateLoop()
			return
		}

		// Initial tick
		err = handler.Checkin()

		// Check in failed
		if err != nil {
			log.Warn("initial check-in failed, verifying service health")

			// TroubleShooter failed, Terminate
			if !verifyServiceHealth(app, err) {
				// Critical fault, set EXIT_CODE = 1 and let systemd restart us
				EXIT_CODE = 1
				TerminateLoop()
				return
			}
		}

		log.Info("task handler check-in completed, marking system as healthy, start polling")
		slot, oerr := app.OtaService.MarkBooted(rauc.SlotStatusGood)
		if oerr != nil {
			log.Error("OTA HealthCheck marking failed, continuing operation", zap.String("slot", slot), zap.Error(oerr))
		}

		for {
			select {
			case <-jobTicker.C:
				systemd.EntertainWatchdog()

				// This is just in case we missed a signal
				if IsRebootPending(false) {
					TerminateLoop()
					return
				}

				// Try to check-in with the server
				err = handler.Checkin()
				if err != nil {
					// Terminate if the troubleshooter found some problem
					if !verifyServiceHealth(app, err) {
						EXIT_CODE = 1
						TerminateLoop()
						return
					}

					continue
				}

				// Signal the job-handler to tick
				handler.Tick()

			case <-app.ReloadSignal:
				log.Info("reload signal received")

				systemd.EntertainWatchdog()

				if IsRebootPending(false) {
					TerminateLoop()
					return
				}

			case <-app.ExitSignal:
				log.Info("exit signal received - shutting down tasks and routines")

				systemd.EntertainWatchdog()
				IsRebootPending(true)
				TerminateLoop()
				return
			}
		}
	}()

	// Wait until everything terminates
	app.WG.Wait()

	log.Info("pending tasks and routines terminated")

	// Shutdown everything
	app.Shutdown()

	// Stop the tasks
	handler.Shutdown()

	// Final greetings :)
	log.Info("stopped observing the sky!")

	// Perform a pending reboot if it is scheduled
	if rebootCMD != nil {
		if err = rebootCMD.Run(); err != nil {
			log.Error("Could not reboot, thats problematic ...", zap.Error(err))
		}
	}

	// Exit with the proper code
	os.Exit(EXIT_CODE)
}
