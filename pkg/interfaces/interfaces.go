package interfaces

import (
	"context"
	"io"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
)

type LRU interface {
	// Add Inserts a value into the LRU. A boolean is returned that indicates if the value was successfully added.
	Add(key, value interface{}) bool

	// PushBack Inserts a value into the back of the LRU. A boolean is returned that indicates if the value was successfully added.
	PushBack(key, value interface{}) bool

	// Get a value from the LRU, returns a boolean indicating if the value was present.
	Get(key interface{}) (interface{}, bool)

	// Contains returns a boolean indicating if the value is present in the LRU.
	Contains(key interface{}) bool

	// Remove removes a value from the LRU, releasing resources associated with that value. Returns a boolean indicating if the value was successfully removed.
	Remove(key interface{}) bool

	// Purge removes all items in the LRU.
	Purge()

	// Size returns the total "size" of the LRU.
	Size() int64

	// RemoveOldest removes the oldest value in the LRU
	RemoveOldest() (interface{}, bool)
}

type Cache interface {
	WithIsolation(ctx context.Context, cacheType CacheType, remoteInstanceName string) (Cache, error)
	// Contains return a boolean indicating if the digest(file) present in cache
	Contains(ctx context.Context, d *repb.Digest) (bool, error)

	// FindMissing  receive a list of digests(files) and return that not exists in cache
	FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error)

	Get(ctx context.Context, d *repb.Digest) ([]byte, error)
	GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error)
	Set(ctx context.Context, d *repb.Digest, data []byte) error
	SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error
	Delete(ctx context.Context, d *repb.Digest) error
	Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error)
	Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error)
	Size() int64
	Check(ctx context.Context) error
}

type CacheType int

const (
	UnknownCacheType CacheType = iota
	ActionCacheType
	CASCacheType
)

func (t CacheType) Prefix() string {
	switch t {
	case ActionCacheType:
		return "ac"
	case CASCacheType:
		return ""
	default:
		return "unknown"
	}
}

// CommandResult captures the output and details of an executed command.
// Copy from buildbuddy server/interfaces/interfaces.go:456
type CommandResult struct {
	// Error is populated only if the command was unable to be started, or if it was
	// started but never completed.
	//
	// In particular, if the command runs and returns a non-zero exit code (such as 1),
	// this is considered a successful execution, and this error will NOT be populated.
	//
	// In some cases, the command may have failed to start due to an issue unrelated
	// to the command itself. For example, the runner may execute the command in a
	// sandboxed environment but fail to create the sandbox. In these cases, the
	// Error field here should be populated with a gRPC error code indicating why the
	// command failed to start, and the ExitCode field should contain the exit code
	// from the sandboxing process, rather than the command itself.
	//
	// If the call to `exec.Cmd#Run` returned -1, meaning that the command was killed or
	// never exited, this field should be populated with a gRPC error code indicating the
	// reason, such as DEADLINE_EXCEEDED (if the command times out), UNAVAILABLE (if
	// there is a transient error that can be retried), or RESOURCE_EXHAUSTED (if the
	// command ran out of memory while executing).
	Error error
	// CommandDebugString indicates the command that was run, for debugging purposes only.
	CommandDebugString string
	// Stdout from the command. This may contain data even if there was an Error.
	Stdout []byte
	// Stderr from the command. This may contain data even if there was an Error.
	Stderr []byte

	// ExitCode is one of the following:
	// * The exit code returned by the executed command
	// * -1 if the process was killed or did not exit
	// * -2 (NoExitCode) if the exit code could not be determined because it returned
	//   an error other than exec.ExitError. This case typically means it failed to start.
	ExitCode int
}
