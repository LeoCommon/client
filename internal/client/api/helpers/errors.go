package helpers

import (
	"fmt"

	"github.com/imroc/req/v3"
)

type ResponseError struct {
	Status string
	Body   []byte
	Code   int
}

// Error converts the response error to string, but does not print body!
func (e *ResponseError) Error() string {
	return fmt.Sprintf("code: %d status: %s", e.Code, e.Status)
}

// ErrrFromResponse provides properly typed errors for further handling
func ErrorFromResponse(err error, resp *req.Response) error {
	// If an error was encountered, relay it unwrapped
	if err != nil {
		return err
	}

	// everything okay
	if resp.IsSuccessState() {
		return nil
	}

	// Check if there was an underlying error
	respErr := resp.Error()
	if respErr != nil {
		return respErr.(error)
	}

	// Default response error
	return &ResponseError{
		Code:   resp.StatusCode,
		Status: resp.Status,
		Body:   resp.Bytes(),
	}
}
