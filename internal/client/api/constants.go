package api

import "time"

const (
	RequestTimeout          = 5 * time.Second
	RequestRetryMinWaitTime = 250 * time.Millisecond
	RequestRetryMaxWaitTime = 3 * time.Second
	MaxRetries              = 3
)
