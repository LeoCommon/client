package main

import (
	"log"
	"time"

	"github.com/tarm/serial"
)

var (
	CRLF = "\r\n"

	SIMTECH_MGMT_PORT     = "/dev/ttyUSB2"
	SIMTECH_MGMT_BAUDRATE = 115200
	AT_SIMTECH_GPS_START  = "AT+CGPS=1,1" + CRLF

	// retrieve using AT+CGPSAUTO? => reply: +CGPSAUTO: 0 or 1
	AT_SIMTECH_GPS_AUTOSTART = "AT+CGPSAUTO=" // add ? or 1

	AT_SIMTECH_GPS_NMEA_RATE = "AT+CGPSNMEARATE="

	SIMTECH_GPS_PORT = "/dev/ttyUSB1"
)

func main() {
	c := &serial.Config{
		Name:        SIMTECH_MGMT_PORT,
		Baud:        SIMTECH_MGMT_BAUDRATE,
		ReadTimeout: time.Second * 5}

	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	n, err := s.Write([]byte(AT_SIMTECH_GPS_AUTOSTART))
	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 128)
	n, err = s.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%q", buf[:n])
}
