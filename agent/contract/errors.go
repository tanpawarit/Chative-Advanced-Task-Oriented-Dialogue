package contract

import "errors"

var (
	ErrModelInvoke     = errors.New("model invoke failed")
	ErrSchemaViolation = errors.New("model response violates schema")
	ErrPromptMissing   = errors.New("required prompt is missing")
	ErrValidation      = errors.New("validation failed")
)
