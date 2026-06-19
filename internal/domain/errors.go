package domain

import "errors"

var ErrAlreadyExists = errors.New("resource already exists")

var ErrNotFound = errors.New("resource not found")

var ErrIncorrectRepositoryFormat = errors.New("incorrect repository format")

var ErrNoReleases = errors.New("repository has no releases")

var ErrRateLimited = errors.New("external api rate limit exceeded")

var ErrInvalidToken = errors.New("invalid token")
