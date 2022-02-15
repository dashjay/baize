package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
)

func CalSHA256OfInput(input []byte) *repb.Digest {
	h := sha256.New()
	h.Write(input)
	return &repb.Digest{Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: int64(len(input))}
}

func CalSHA256FromReader(r io.Reader) (*repb.Digest, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return nil, err
	}
	return &repb.Digest{Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: n}, nil
}

func RandomBytes(byteLength int) []byte {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	length := len(letters)
	output := make([]byte, byteLength)
	if _, err := rand.Read(output); err != nil {
		panic(err)
	}
	// Run through output; replacing each with the equivalent random char.
	for i, b := range output {
		output[i] = letters[b%byte(length)]
	}
	return output
}

func RandomString(stringLength int) string {
	return string(RandomBytes(stringLength))
}
