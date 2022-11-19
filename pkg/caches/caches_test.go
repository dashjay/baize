package caches

import (
	"context"
	"io/ioutil"
	"os"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils"
)

const defaultRandomBytesSize = 200

var cache interfaces.Cache

var _ = Describe("test all kind of cache", func() {
	var (
		ctx         = context.Background()
		err         error
		tempdir     string
		originCache interfaces.Cache
	)
	BeforeEach(func() {
		tempdir, err = ioutil.TempDir(os.TempDir(), "")
		Expect(err).To(BeNil())
	})
	JustBeforeEach(func() {
		cache, err = originCache.WithIsolation(context.Background(), interfaces.ActionCacheType, "test")
		Expect(err).To(BeNil())
	})
	AfterEach(func() {
		Expect(os.RemoveAll(tempdir)).To(BeNil())
	})
	Context("disk cache test", func() {
		BeforeEach(func() {
			originCache = NewDiskCache(&cc.Cache{
				Enabled:   true,
				CacheSize: 65535,
				CacheAddr: tempdir,
			})
		})
		RunAllTest(ctx)
	})
	Context("composed cache test", func() {
		BeforeEach(func() {
			a := NewDiskCache(&cc.Cache{
				CacheSize: 65535,
				CacheAddr: tempdir,
			})
			b := NewMemoryCache(&cc.Cache{
				CacheSize: 65535,
			})
			originCache = NewComposedCache(a, b, ModeReadThrough|ModeWriteThrough)
		})
		RunAllTest(ctx)
	})
	Context("memory cache", func() {
		BeforeEach(func() {
			originCache = NewMemoryCache(&cc.Cache{
				CacheSize: 1024 * 1024 * 1024,
			})
		})
		RunAllTest(ctx)
	})
})

func RunAllTest(ctx context.Context) {
	It("Get and Set", func() {
		src := utils.RandomBytes(defaultRandomBytesSize)
		digest := utils.CalSHA256OfInput(src)
		Expect(cache.Set(ctx, digest, src)).To(BeNil())
		got, err := cache.Get(ctx, digest)
		Expect(err).To(BeNil())
		Expect(got).To(Equal(src))
	})
	It("SetMulti and GetMulti", func() {
		src := [3][]byte{utils.RandomBytes(defaultRandomBytesSize), utils.RandomBytes(defaultRandomBytesSize), utils.RandomBytes(defaultRandomBytesSize)}
		digestMap := make(map[*repb.Digest][]byte)
		for i := range src {
			digestMap[utils.CalSHA256OfInput(src[i])] = src[i]
		}
		Expect(cache.SetMulti(ctx, digestMap)).To(BeNil())
		var s []*repb.Digest
		for k := range digestMap {
			s = append(s, k)
		}
		output, err := cache.GetMulti(ctx, s)
		Expect(err).To(BeNil())
		for k := range output {
			Expect(output[k]).To(Equal(digestMap[k]))
		}
	})
	It("Set And Remove", func() {
		src := utils.RandomBytes(defaultRandomBytesSize)
		digest := utils.CalSHA256OfInput(src)
		Expect(cache.Set(ctx, digest, src)).To(BeNil())
		Expect(cache.Delete(ctx, digest)).To(BeNil())
		exists, err := cache.Contains(ctx, digest)
		Expect(err).To(BeNil())
		Expect(exists).To(Equal(false))
	})
	It("SetMulti and FindMissing", func() {
		src := [3][]byte{utils.RandomBytes(defaultRandomBytesSize), utils.RandomBytes(defaultRandomBytesSize), utils.RandomBytes(defaultRandomBytesSize)}
		digestMap := make(map[*repb.Digest][]byte)

		digestMap[utils.CalSHA256OfInput(src[0])] = src[0]
		digestOfSec := utils.CalSHA256OfInput(src[1])
		digestMap[utils.CalSHA256OfInput(src[2])] = src[2]

		Expect(cache.SetMulti(ctx, digestMap)).To(BeNil())
		var s []*repb.Digest
		for k := range digestMap {
			s = append(s, k)
		}
		s = append(s, digestOfSec)
		output, err := cache.FindMissing(ctx, s)
		Expect(err).To(BeNil())
		Expect(len(output)).To(Equal(1))
		Expect(digestOfSec).To(Equal(output[0]))
	})
	It("Writer and Reader", func() {
		src := utils.RandomBytes(defaultRandomBytesSize)
		digest := utils.CalSHA256OfInput(src)
		w, err := cache.Writer(ctx, digest)
		Expect(err).To(BeNil())
		_, err = w.Write(src)
		Expect(err).To(BeNil())
		Expect(w.Close()).To(BeNil())
		r, err := cache.Reader(ctx, digest, 0)
		Expect(err).To(BeNil())
		content, err := ioutil.ReadAll(r)
		Expect(err).To(BeNil())
		Expect(r.Close()).To(BeNil())
		Expect(content).To(Equal(content))
	})
}
