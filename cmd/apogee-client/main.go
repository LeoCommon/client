package main

import (
	"fmt"
	"os"
	"time"

	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/apogee"
	"disco.cs.uni-kl.de/apogee/pkg/system/cli"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/net"
	"disco.cs.uni-kl.de/apogee/pkg/system/services/rauc"
	"disco.cs.uni-kl.de/apogee/pkg/task/handler"
	"github.com/google/uuid"
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
	WIFIPSK := ""

	// Register the job handler
	handler, err := handler.NewJobHandler(app)

	if err != nil {
		apglog.Fatal("Could not start job handler, aborting", zap.Error(err))
		return
	}

	// Run the application mainloop (blocking)
	//err = app.NetworkService.EnforceNetworkPriority()

	gsmUUID, err := uuid.Parse("6d9a50b4-583b-476a-b3d2-9b282cdbff74")
	if err != nil {
		apglog.Fatal("UUID invalid", zap.Error(err))
	}

	wifiUUID, err := uuid.Parse("5436ccb9-f30e-48f4-8d87-aa3ff00f43a8")
	if err != nil {
		apglog.Fatal("UUID invalid", zap.Error(err))
	}

	conf := net.NewGSMNetworkConfig("internet.telekom", "congstar", "cs")
	conf.WithName("TestMyGSMConnection").WithUUID(&gsmUUID).WithV4Automatic().WithV6Automatic()
	err = app.NetworkService.CreateConnection(conf)
	apglog.Warn("GSM test terminated", zap.Error(err))

	// Tests invalid device name
	wconf := net.NewWirelessNetworkConfig("IS15GIOT", WIFIPSK)
	// Test invalid device name
	wconf.WithDeviceName("wifi1").WithName("TestDHCPInvalidDeviceWifi").WithUUID(&wifiUUID).WithV4Automatic().WithV6Automatic()

	err = app.NetworkService.CreateConnection(wconf)
	apglog.Warn("WiFi test terminated", zap.Error(err))

	// Test valid configuration
	wconf = net.NewWirelessNetworkConfig("IS15GIOT", WIFIPSK)
	wconf.WithName("TestStaticWiFiConnection").WithUUID(&wifiUUID)
	// Set static v4
	wconf.WithV4Static(net.V4Config{
		Static: &net.Static{
			Address: "10.0.1.165",
			Gateway: "10.0.1.1",
			Prefix:  24,
		},
	}).WithV6Automatic().WithCustomDNS([]string{"10.0.1.1", "fe80::2e2:69ff:fe5c:5dfe"})

	err = app.NetworkService.CreateConnection(wconf)
	apglog.Warn("WiFi test terminated", zap.Error(err))

	// Test valid configuration with duplicate UUID
	wconf = net.NewWirelessNetworkConfig("IS15GIOT", WIFIPSK)
	wconf.WithName("TestDHCPValidDeviceWifi").WithUUID(&wifiUUID)
	// Test valid device name
	wconf.WithDeviceName("wlan0")
	wconf.WithV4Automatic().WithV6Automatic()

	err = app.NetworkService.CreateConnection(wconf)
	apglog.Fatal("WiFi test terminated", zap.Error(err))

	// At this point the app struct is filled, and we can use it
	clientConfig := app.Config.Client
	jobTicker := time.NewTicker(time.Duration(clientConfig.PollingInterval) * time.Second)
	app.WG.Add(1)

	EXIT_CODE := 0

	// Attention: "tick shifts"
	// If the execution takes more time, consequent runs are delayed.
	go func() {
		RebootRequired := func(skipHandler bool) bool {
			// Check if the reboot marker exists and if we can safely reboot
			if rebootMarkerExists() {
				apglog.Info("Reboot marker detected")

				if skipHandler || !handler.HasRunningJob() {
					apglog.Info("Going to soft-reboot now", zap.Bool("SkipHandler", skipHandler))
					err := cli.SoftReboot()
					if err == nil {
						return true
					}

					apglog.Error("Could not reboot, thats problematic ...", zap.Error(err))
				}
			}

			return false
		}

		TerminateLoop := func() {
			jobTicker.Stop()
			app.WG.Done()
		}

		// Check if we have an imminent reboot this early
		if RebootRequired(true) {
			apglog.Info("Skipping checkin, terminating early ...")
			TerminateLoop()
			return
		}

		// Initial tick
		err := handler.Checkin()

		// Check but the connectivity checker deemed it non-critical
		if err != nil {
			apglog.Warn("initial check-in failed, running troubleshooter", zap.Error(err))

			// If the troubleshooter confirms, we have to terminate
			if TroubleShootConnectivity(app, err) {
				EXIT_CODE = 1
				TerminateLoop()
			}

			// Critical fault, set EXIT_CODE = 1 and let systemd restart us
			EXIT_CODE = 1
			TerminateLoop()
			return
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
				if RebootRequired(false) {
					TerminateLoop()
					return
				}

				// Signal the job-handler to tick
				err := handler.Checkin()
				if err != nil {
					TroubleShootConnectivity(app, err)
					continue
				}

				handler.Tick()

			case <-app.ReloadSignal:
				apglog.Info("reload signal received")

				EntertainWatchdog()

				if RebootRequired(false) {
					TerminateLoop()
					return
				}

			case <-app.ExitSignal:
				apglog.Info("exit signal received - shutting down tasks and routines")

				EntertainWatchdog()

				RebootRequired(true)
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

	// Just making sure we exit with code 0, so we dont get re-started by systemd
	os.Exit(EXIT_CODE)
}
