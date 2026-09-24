package collector

import (
	"io"
	"log"
)

// A response-body close failure cannot change an already decoded result, but
// retaining the error in logs makes connection cleanup failures observable.
func closeResponseBody(body io.ReadCloser) {
	if err := body.Close(); err != nil {
		log.Printf("close upstream response body: %v", err)
	}
}
