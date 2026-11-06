package users

import "errors"

var (
	ErrMissingFields      = errors.New("email and password are required")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("incorrect email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
)
