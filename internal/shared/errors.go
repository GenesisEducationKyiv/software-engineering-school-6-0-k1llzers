package shared

import "errors"

var ErrNotFound = errors.New("resource not found")

var ErrRateLimited = errors.New("external api rate limit exceeded")
