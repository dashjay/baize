package server

const (
	// Instance-related constants
	DefaultInstanceName = ""

	// Nil-data/Empty SHA-256 data
	EmptySha  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	EmptySize = int64(0)

	// Resource naming constants
	ResourceNameType   = "blobs"
	ResourceNameAction = "uploads"

	// Default buffer sizes
	DefaultReadCapacity = 1024 * 1024
)