package udev

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pilebones/go-udev/crawler"
	"github.com/pilebones/go-udev/netlink"
)

var (
	filePath              *string
	monitorMode, infoMode *bool
)

func RunMain() {

	rules := getUSBMatchRules()

	info(&rules)

	monitor(&rules)

}

func info(matcher netlink.Matcher) {
	log.Println("Get existing devices...")

	queue := make(chan crawler.Device)
	errors := make(chan error)
	quit := crawler.ExistingDevices(queue, errors, matcher)

	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting info mode...")
		close(quit)
		os.Exit(0)
	}()

	// Handling message from queue
	for {
		select {
		case device, more := <-queue:
			if !more {
				log.Println("Finished processing existing devices")
				return
			}
			log.Println("Detect device at", device.KObj, "with env", device.Env)
		case err := <-errors:
			log.Println("ERROR:", err)
		}
	}
}

// monitor run monitor mode
func monitor(matcher netlink.Matcher) {
	log.Println("Monitoring UEvent kernel message to user-space...")

	conn := new(netlink.UEventConn)
	if err := conn.Connect(netlink.UdevEvent); err != nil {
		log.Fatalln("Unable to connect to Netlink Kobject UEvent socket")
	}
	defer conn.Close()

	queue := make(chan netlink.UEvent)
	errors := make(chan error)
	quit := conn.Monitor(queue, errors, matcher)

	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting monitor mode...")
		close(quit)
		os.Exit(0)
	}()

	// Handling message from queue
	for {
		select {
		case uevent := <-queue:
			log.Println("Handle", uevent)
		case err := <-errors:
			log.Println("ERROR:", err)
		}
	}

}

// getOptionnalMatcher Parse and load config file which contains rules for matching
func getUSBMatchRules() netlink.RuleDefinitions {

	remove := netlink.REMOVE.String()

	const (
		ADD_REGEX netlink.KObjAction = "^ad+$"
		USB_MATCH string             = "^usb.*"
	)

	add := ADD_REGEX.String()

	/*
		ENV_SUBSYS_USB := map[string]string{
			"SUBSYSTEM": USB_MATCH,
		}*/

	ENV_SUBSYS_BLOCK := map[string]string{
		"SUBSYSTEM": "block",
		"DEVTYPE":   "partition",
	}

	return netlink.RuleDefinitions{Rules: []netlink.RuleDefinition{
		{
			Action: &add,
			Env:    ENV_SUBSYS_BLOCK,
		},

		{
			Action: &remove,
			Env:    ENV_SUBSYS_BLOCK,
		},
	}}
}
