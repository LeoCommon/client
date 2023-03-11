package api

import "time"

const (
	RequestTimeout          = 1 * time.Second
	RequestRetryWaitTime    = 250 * time.Millisecond
	RequestRetryMaxWaitTime = 3 * time.Second
	MaxRetries              = 3
)
