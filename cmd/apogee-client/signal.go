package main

import (
	"os"

	"disco.cs.uni-kl.de/apogee/pkg/constants"
)

func rebootMarkerExists() bool {
	_, err := os.Stat(constants.REBOOT_PENDING_TMPFILE)
	return !os.IsNotExist(err)
}
