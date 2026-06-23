package subscriptions

import "errors"

var ErrAlreadyExists = errors.New("resource already exists")

var ErrIncorrectRepositoryFormat = errors.New("incorrect repository format")

var ErrInvalidToken = errors.New("invalid token")

var ErrQuotaRejected = errors.New("subscription quota rejected")
