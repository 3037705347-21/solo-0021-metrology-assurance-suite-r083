package domain

import "errors"

var (
	ErrInvalidInput         = errors.New("invalid input")
	ErrNotFound             = errors.New("resource not found")
	ErrDuplicateAssetTag    = errors.New("duplicate asset tag")
	ErrDuplicateReference   = errors.New("duplicate reference lot")
	ErrActiveCaseExists     = errors.New("device already has an unfinished case")
	ErrStateConflict        = errors.New("resource state conflict")
	ErrSealAlreadyCreated   = errors.New("case already has an evidence seal")
	ErrReferenceUnknown     = errors.New("reference batch is not registered")
	ErrReferenceExpired     = errors.New("reference batch is expired")
	ErrReferenceExhausted   = errors.New("reference batch allowance is exhausted")
	ErrReferenceCheckFailed = errors.New("reference batch verification check failed")
)

func IsKnownError(err error) bool {
	return errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrDuplicateAssetTag) ||
		errors.Is(err, ErrDuplicateReference) ||
		errors.Is(err, ErrActiveCaseExists) ||
		errors.Is(err, ErrStateConflict) ||
		errors.Is(err, ErrSealAlreadyCreated) ||
		errors.Is(err, ErrReferenceUnknown) ||
		errors.Is(err, ErrReferenceExpired) ||
		errors.Is(err, ErrReferenceExhausted) ||
		errors.Is(err, ErrReferenceCheckFailed)
}
