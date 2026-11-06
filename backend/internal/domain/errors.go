package domain

import "errors"

// ErrDuplicateRequest is returned by a repository Insert when the request key was already used.
var ErrDuplicateRequest = errors.New("lời mời này đã được gửi hoặc đang gửi")
