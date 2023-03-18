package handler

import "time"

const (
	MaxJobDuration = 24 * time.Hour  // Maximum job duration
	EditDeadline   = 1 * time.Second // Specifies the minimal amount of time pre-start where editing jobs is still possible
)
