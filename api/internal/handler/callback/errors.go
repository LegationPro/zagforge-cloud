package callback

import "errors"

var (
	ErrInvalidRequestBody  = errors.New("invalid request body")
	ErrFailedToReadBody    = errors.New("failed to read body")
	ErrJobIDMismatch       = errors.New("job_id mismatch")
	ErrInvalidJobID        = errors.New("invalid job_id")
	ErrInvalidStatus       = errors.New("status must be 'succeeded' or 'failed'")
	ErrJobNotFound         = errors.New("job not found")
	ErrJobAlreadyTerminal  = errors.New("job already in terminal state")
	ErrInternal            = errors.New("internal error")
	ErrFailedToCloneToken  = errors.New("failed to generate clone token")
)
