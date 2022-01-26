package caches

import (
	"errors"
	"fmt"
)

type errNotFound struct {
	key string
}

var errByteSizeOverCutoffSize = errors.New("the value's size has more than defaultCutoffSizeBytes")

func (e errNotFound) Error() string {
	return fmt.Sprintf("key %q not found", e.key)
}

func IsNotFoundError(err error) bool {
	_, ok := err.(errNotFound)
	return ok
}
