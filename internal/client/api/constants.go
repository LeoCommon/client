package api

import "time"

const (
	RequestTimeout          = 1 * time.Second
	RequestRetryWaitTime    = 10 * time.Second
	RequestRetryMaxWaitTime = 10 * time.Second
	MaxRetries              = 3
)
