package application

import "errors"

var (
	ErrNotFound            = errors.New("campaign not found")
	ErrVersionConflict     = errors.New("expected version conflict")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different request")
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type AuthorizationError struct{ Message string }

func (e *AuthorizationError) Error() string { return e.Message }
