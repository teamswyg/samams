package shared

import "fmt"

// DomainError is the marker interface for all domain-level errors.
// It lets upper layers distinguish between validation/conflict/not-found, etc.

type DomainError interface {
	error
	DomainError()
}

type ValidationError struct {
	Msg string
}

func (e ValidationError) Error() string { return e.Msg }
func (ValidationError) DomainError()    {}

type NotFoundError struct {
	Resource string
	ID       string
}

func (e NotFoundError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("%s not found", e.Resource)
	}
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

func (NotFoundError) DomainError() {}

type ConflictError struct {
	Msg string
}

func (e ConflictError) Error() string { return e.Msg }
func (ConflictError) DomainError()    {}

