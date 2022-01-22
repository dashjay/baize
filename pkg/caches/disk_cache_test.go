package caches

import (
	"bytes"
	"context"
	"github.com/dashjay/bazel-remote-exec/pkg/utils"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	tempdir := t.TempDir()
	t.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)
	dc := NewDiskCache(tempdir, 65536)
	ctx := context.Background()
	rBytes := utils.RandomBytes(400)
	err := dc.Set(ctx, utils.CalSHA256OfInnput(rBytes), rBytes)
	if err != nil {
		t.Error(err)
		return
	}
	b, err := dc.Get(ctx, utils.CalSHA256OfInnput(rBytes))
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
		prepare[i] = utils.RandomBytes(400)
	}

	t.Run("Set", func(b *testing.B) {
		b.StartTimer()
		ctx := context.Background()
		for i := 0; i < len(prepare); i++ {
			err := dc.Set(ctx, utils.CalSHA256OfInnput(prepare[i]), prepare[i])
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = dc.Get(ctx, utils.CalSHA256OfInnput(prepare[i]))
		}
		b.StopTimer()
	})

	t.Logf("Current Size: %d", dc.lru.Size())
}
