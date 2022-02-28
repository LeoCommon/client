package main

import (
	"disco.cs.uni-kl.de/apogee/pkg/apglog"
	"disco.cs.uni-kl.de/apogee/pkg/modem/sim7600"
	"disco.cs.uni-kl.de/apogee/pkg/modem/sim7600/atparser"
)

var (
	DEBUG = true
)

func main() {
	SIM7600Modem := sim7600.Create(nil)

	SIM7600Modem.Open()
	err := SIM7600Modem.StartGPS(atparser.GPS_MODE_STANDALONE)
	if err != nil {
		apglog.Fatal(err.Error())
	}

	/*err = SIM7600Modem.Reset()
	if err != nil {
		apglog.Fatal(err.Error())
	}*/
}
