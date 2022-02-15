package digest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/google/uuid"

	"github.com/dashjay/baize/pkg/utils/status"
)

var (
	uploadRegex = regexp.MustCompile(`^(?:(?:(?P<instance_name>.*)/)?uploads/(?P<uuid>[a-f0-9-]{36})/)?(?P<blob_type>blobs|compressed-blobs/zstd)/(?P<hash>[a-f0-9]{64})/(?P<size>\d+)`)
)

type ResourceName struct {
	digest       *repb.Digest
	instanceName string
	compressor   repb.Compressor_Value
}

func NewResourceName(d *repb.Digest, instanceName string) *ResourceName {
	return &ResourceName{
		digest:       d,
		instanceName: instanceName,
		compressor:   repb.Compressor_IDENTITY,
	}
}

func (r *ResourceName) GetDigest() *repb.Digest {
	return r.digest
}

func (r *ResourceName) GetInstanceName() string {
	return r.instanceName
}

func (r *ResourceName) GetCompressor() repb.Compressor_Value {
	return r.compressor
}

func (r *ResourceName) SetCompressor(compressor repb.Compressor_Value) {
	r.compressor = compressor
}

// DownloadString returns a string representing the resource name for download
// purposes.
func (r *ResourceName) DownloadString() string {
	// Normalize slashes, e.g. "//foo/bar//"" becomes "/foo/bar".
	instanceName := filepath.Join(filepath.SplitList(r.GetInstanceName())...)
	return fmt.Sprintf(
		"%s/%s/%s/%d",
		instanceName, blobTypeSegment(r.GetCompressor()),
		r.GetDigest().GetHash(), r.GetDigest().GetSizeBytes())
}

// UploadString returns a string representing the resource name for upload
// purposes.
func (r *ResourceName) UploadString() (string, error) {
	// Normalize slashes, e.g. "//foo/bar//"" becomes "/foo/bar".
	instanceName := filepath.Join(filepath.SplitList(r.GetInstanceName())...)
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"%s/uploads/%s/%s/%s/%d",
		instanceName, u.String(), blobTypeSegment(r.GetCompressor()),
		r.GetDigest().GetHash(), r.GetDigest().GetSizeBytes(),
	), nil
}

func blobTypeSegment(compressor repb.Compressor_Value) string {
	if compressor == repb.Compressor_ZSTD {
		return "compressed-blobs/zstd"
	}
	return "blobs"
}

func ParseUploadResourceName(resourceName string) (*ResourceName, error) {
	return parseResourceName(resourceName, uploadRegex)
}
func parseResourceName(resourceName string, matcher *regexp.Regexp) (*ResourceName, error) {
	match := matcher.FindStringSubmatch(resourceName)
	result := make(map[string]string, len(match))
	for i, name := range matcher.SubexpNames() {
		if i != 0 && name != "" && i < len(match) {
			result[name] = match[i]
		}
	}
	hash, hashOK := result["hash"]
	sizeStr, sizeOK := result["size"]
	if !hashOK || !sizeOK {
		return nil, status.InvalidArgumentErrorf("Unparsable resource name: %s", resourceName)
	}
	if hash == "" {
		return nil, status.InvalidArgumentErrorf("Unparsable resource name (empty hash?): %s", resourceName)
	}
	sizeBytes, err := strconv.ParseInt(sizeStr, 10, 0)
	if err != nil {
		return nil, err
	}

	// Set the instance name, if one was present.
	instanceName := ""
	if in, ok := result["instance_name"]; ok {
		instanceName = in
	}

	// Determine compression level from blob type segment
	blobTypeStr, sizeOK := result["blob_type"]
	if !sizeOK {
		// Should never happen since the regex would not match otherwise.
		return nil, status.InvalidArgumentError(`Unparsable resource name: "/blobs" or "/compressed-blobs/zstd" missing or out of place`)
	}
	compressor := repb.Compressor_IDENTITY
	if blobTypeStr == "compressed-blobs/zstd" {
		compressor = repb.Compressor_ZSTD
	}
	d := &repb.Digest{Hash: hash, SizeBytes: sizeBytes}
	r := NewResourceName(d, instanceName)
	r.SetCompressor(compressor)
	return r, nil
}
