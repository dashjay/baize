package interfaces

import (
	"context"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"io"
)

type LRU interface {
	// Add Inserts a value into the LRU. A boolean is returned that indicates if the value was successfully added.
	Add(key string, value interface{}) bool

	// PushBack Inserts a value into the back of the LRU. A boolean is returned that indicates if the value was successfully added.
	PushBack(key string, value interface{}) bool

	// Get a value from the LRU, returns a boolean indicating if the value was present.
	Get(key string) (interface{}, bool)

	// Contains returns a boolean indicating if the value is present in the LRU.
	Contains(key string) bool

	// Remove removes a value from the LRU, releasing resources associated with that value. Returns a boolean indicating if the value was sucessfully removed.
	Remove(key string) bool

	// Purge removes all items in the LRU.
	Purge()

	// Size returns the total "size" of the LRU.
	Size() int64

	// RemoveOldest removes the oldest value in the LRU
	RemoveOldest() (interface{}, bool)
}

type Cache interface {
	Contains(ctx context.Context, d *repb.Digest) (bool, error)
	FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error)
	Get(ctx context.Context, d *repb.Digest) ([]byte, error)
	GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error)
	Set(ctx context.Context, d *repb.Digest, data []byte) error
	SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error
	Delete(ctx context.Context, d *repb.Digest) error
	Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error)
	Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error)
}
