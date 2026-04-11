package domain

import "errors"

var ErrAlreadyExists = errors.New("resource already exists")

var ErrNotFound = errors.New("resource not found")
