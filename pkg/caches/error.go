package caches

import (
	"errors"
)

var errByteSizeOverCutoffSize = errors.New("the value's size has more than defaultCutoffSizeBytes")
