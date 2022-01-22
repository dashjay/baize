package caches

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	tempdir := t.TempDir()
	t.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)
	dc := NewDiskCache(tempdir, 65536)
	ctx := context.Background()
	rBytes := randomBytes(400)
	err := dc.Set(ctx, genSha256(rBytes), rBytes)
	if err != nil {
		t.Error(err)
		return
	}
	b, err := dc.Get(ctx, genSha256(rBytes))
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(rBytes, b) {
		t.Errorf("two bytes not equal, len(a) = %d, len(b) = %d", len(rBytes), len(b))
	}
}
func BenchmarkDiskCache(t *testing.B) {
	tempdir := t.TempDir()
	t.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)

	dc := NewDiskCache(tempdir, 65535)

	const LENGTH = 4000
	var prepare = [LENGTH][]byte{}
	for i := 0; i < LENGTH; i++ {
		prepare[i] = randomBytes(400)
	}

	t.Run("Set", func(b *testing.B) {
		b.StartTimer()
		ctx := context.Background()
		for i := 0; i < len(prepare); i++ {
			err := dc.Set(ctx, genSha256(prepare[i]), prepare[i])
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = dc.Get(ctx, genSha256(prepare[i]))
		}
		b.StopTimer()
	})

	t.Logf("Current Size: %d", dc.lru.Size())
}

func genSha256(input []byte) *repb.Digest {
	h := sha256.New()
	h.Write(input)
	return &repb.Digest{Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: int64(len(input))}
}

func randomBytes(stringLength int) []byte {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	length := len(letters)
	output := make([]byte, stringLength)
	if _, err := rand.Read(output); err != nil {
		panic(err)
	}
	// Run through output; replacing each with the equivalent random char.
	for i, b := range output {
		output[i] = letters[b%byte(length)]
	}
	return output
}
