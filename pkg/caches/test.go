package caches

import (
	"context"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"github.com/dashjay/bazel-remote-exec/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCache(m interfaces.Cache, t *testing.T) {
	ctx := context.Background()
	t.Run("Set and Get", func(t *testing.T) {
		src := utils.RandomBytes(1024)
		digest := utils.CalSHA256OfInnput(src)
		err := m.Set(ctx, digest, src)
		if err != nil {
			t.Error(err)
			return
		}
		got, err := m.Get(ctx, digest)
		if err != nil {
			t.Error(err)
		}
		assert.Equal(t, got, src)
	})

	t.Run("SetMulti and GetMulti", func(t *testing.T) {
		src := [3][]byte{utils.RandomBytes(1024), utils.RandomBytes(1024), utils.RandomBytes(1024)}
		digestMap := make(map[*repb.Digest][]byte)
		for i := range src {
			digestMap[utils.CalSHA256OfInnput(src[i])] = src[i]
		}
		err := m.SetMulti(ctx, digestMap)
		if err != nil {
			t.Error(err)
			return
		}
		var s []*repb.Digest
		for k := range digestMap {
			s = append(s, k)
		}
		output, err := m.GetMulti(ctx, s)
		if err != nil {
			t.Error(err)
			return
		}
		for k := range output {
			assert.Equal(t, output[k], digestMap[k])
		}
	})

	t.Run("Set And Remove", func(t *testing.T) {
		src := utils.RandomBytes(1024)
		digest := utils.CalSHA256OfInnput(src)
		err := m.Set(ctx, digest, src)
		if err != nil {
			t.Error(err)
			return
		}
		err = m.Delete(ctx, digest)
		if err != nil {
			t.Error(err)
			return
		}
		exists, err := m.Contains(ctx, digest)
		if err != nil {
			t.Error(err)
			return
		}
		assert.Equal(t, false, exists)
	})

	t.Run("SetMulti and FindMissing", func(t *testing.T) {
		src := [3][]byte{utils.RandomBytes(1024), utils.RandomBytes(1024), utils.RandomBytes(1024)}
		digestMap := make(map[*repb.Digest][]byte)
		digestMap[utils.CalSHA256OfInnput(src[0])] = src[0]

		digestOfSec := utils.CalSHA256OfInnput(src[1])

		digestMap[utils.CalSHA256OfInnput(src[2])] = src[2]
		err := m.SetMulti(ctx, digestMap)
		if err != nil {
			t.Error(err)
			return
		}
		var s []*repb.Digest
		for k := range digestMap {
			s = append(s, k)
		}
		s = append(s, digestOfSec)
		output, err := m.FindMissing(ctx, s)
		if err != nil {
			t.Error(err)
			return
		}
		assert.Equal(t, len(output), 1)
		assert.Equal(t, output[0], digestOfSec)
	})
}

func BenchmarkCache(m interfaces.Cache, b *testing.B) {
	ctx := context.Background()
	b.Run("Set and Get", func(bb *testing.B) {
		var src = make([][]byte, bb.N)
		for i := 0; i < bb.N; i++ {
			src[i] = utils.RandomBytes(2048)
		}
		bb.StartTimer()
		for i := 0; i < bb.N; i++ {
			dig := utils.CalSHA256OfInnput(src[i])
			err := m.Set(ctx, dig, src[i])
			if err != nil {
				bb.Error(err)
				return
			}
			_, err = m.Get(ctx, dig)
			if err != nil {
				bb.Error(err)
			}
		}
		bb.StopTimer()
	})

	b.Run("SetMulti and GetMulti", func(bb *testing.B) {
		src := make([][]byte, bb.N)
		for i := range src {
			src[i] = utils.RandomBytes(1024)
		}
		digestMap := make(map[*repb.Digest][]byte, bb.N)
		for i := range src {
			digestMap[utils.CalSHA256OfInnput(src[i])] = src[i]
		}
		bb.StartTimer()
		err := m.SetMulti(ctx, digestMap)
		if err != nil {
			b.Error(err)
			return
		}
		var s []*repb.Digest
		for k := range digestMap {
			s = append(s, k)
		}
		output, err := m.GetMulti(ctx, s)
		if err != nil {
			b.Error(err)
			return
		}
		for k := range output {
			assert.Equal(b, output[k], digestMap[k])
		}
		bb.StopTimer()
	})

	b.Run("SetMulti And RemoveAll", func(bb *testing.B) {
		src := make([][]byte, bb.N)
		for i := range src {
			src[i] = utils.RandomBytes(1024)
		}
		digestMap := make(map[*repb.Digest][]byte, bb.N)
		for i := range src {
			digestMap[utils.CalSHA256OfInnput(src[i])] = src[i]
		}
		bb.StartTimer()
		err := m.SetMulti(ctx, digestMap)
		if err != nil {
			bb.Error(err)
			return
		}
		for k := range digestMap {
			err = m.Delete(ctx, k)
			if err != nil {
				bb.Error(err)
			}
		}
	})
}
