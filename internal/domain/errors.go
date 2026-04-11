package domain

import "errors"

var ErrAlreadyExists = errors.New("resource already exists")

var ErrNotFound = errors.New("resource not found")

var ErrIncorrectRepositoryFormat = errors.New("incorrect repository format")
