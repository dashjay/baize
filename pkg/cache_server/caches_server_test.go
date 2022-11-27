package cache_server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/caches"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

func TestCacheServer(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	logrus.SetLevel(logrus.TraceLevel)
	RegisterFailHandler(Fail)
	RunSpecs(t, "caches suite test")
}

var _ = Describe("test cache server", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		grpcServer *grpc.Server
		err        error
		tempdir    string
		cache      interfaces.Cache
		server     *Server
	)
	BeforeEach(func() {
		tempdir, err = os.MkdirTemp(os.TempDir(), "")
		Expect(err).To(BeNil())
		ctx, cancel = context.WithCancel(context.Background())
		grpcServer = grpc.NewServer()
	})
	AfterEach(func() {
		logrus.Infof("remove tempdir %s", tempdir)
		Expect(os.RemoveAll(tempdir)).To(BeNil())
		grpcServer.GracefulStop()
		cancel()
	})
	It("cache server with diskcache", func() {
		cache = caches.NewMemoryCache(&cc.Cache{
			Enabled:   true,
			CacheSize: 65535,
			CacheAddr: tempdir,
		})
		server = New(cache)

		conn, err := net.Listen("tcp", ":8080")
		Expect(err).To(BeNil())
		bytestream.RegisterByteStreamServer(grpcServer, server)
		go grpcServer.Serve(conn)
		grpcConn, err := grpc.DialContext(ctx, ":8080", grpc.WithInsecure())
		Expect(err).To(BeNil())
		bytestramClient := bytestream.NewByteStreamClient(grpcConn)

		testWriteReadTinyObj(ctx, bytestramClient)

	})
})

func testWriteReadTinyObj(ctx context.Context, cli bytestream.ByteStreamClient) {
	tinyObject := bytes.Repeat([]byte("tiny"), 150)
	sha := sha256.New()
	_, err := sha.Write(tinyObject)
	Expect(err).To(BeNil())
	sha256Sum := hex.EncodeToString(sha.Sum(nil))
	utils.WriteToBytestreamServer(ctx, cli, &repb.Digest{Hash: sha256Sum, SizeBytes: int64(len(tinyObject))}, "", bytes.NewReader(tinyObject))
}
