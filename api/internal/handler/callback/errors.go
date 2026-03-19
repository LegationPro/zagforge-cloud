package callback

import "errors"

var (
	ErrInvalidRequestBody = errors.New("invalid request body")
	ErrJobIDMismatch      = errors.New("job_id mismatch")
	ErrInvalidJobID       = errors.New("invalid job_id")
	ErrJobNotFound        = errors.New("job not found")
	ErrJobAlreadyTerminal = errors.New("job already in terminal state")
	ErrInternal           = errors.New("internal error")
	ErrFailedToCloneToken = errors.New("failed to generate clone token")
)
