package remotecacheutils

import (
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestParse(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	RegisterFailHandler(Fail)
	RunSpecs(t, "caches suite test")
}

const testDigest = "ff2aafd65230a3837fda01a56177d14566fd960c6ec4753d826ef7a0f078a43f"

var _ = Describe("test parse url", func() {
	var testList = []CacheAction{
		{CacheType: "unknown", Digest: testDigest, InstanceName: "default"},
		{CacheType: "unknown", Digest: testDigest},
		{CacheType: "unknown", Digest: testDigest},
		{CacheType: "ac", Digest: testDigest, InstanceName: "default"},
		{CacheType: "ac", Digest: testDigest},
		{CacheType: "ac", Digest: testDigest},
		{CacheType: "cas", Digest: testDigest, InstanceName: "default"},
		{CacheType: "cas", Digest: testDigest},
		{CacheType: "cas", Digest: testDigest},
	}

	test := func(src *CacheAction) {
		dst := Parse(src.String())
		Expect(dst.Digest).To(Equal(src.Digest))
		Expect(dst.InstanceName).To(Equal(src.InstanceName))
		Expect(dst.CacheType).To(Equal(src.CacheType))
	}
	It("build and parse item", func() {
		for i := range testList {
			test(&testList[i])
		}
	})

	It("test parse item", func() {
		ac := Parse("/ac/digest")
		Expect(ac.CacheType).To(Equal("ac"))
		Expect(ac.InstanceName).To(Equal("default"))
		Expect(ac.Digest).To(Equal("digest"))
	})
})
