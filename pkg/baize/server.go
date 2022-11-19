package baize

import (
	"context"
	"math"
	"net"

	"github.com/dashjay/baize/pkg/caches"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/bazelbuild/remote-apis/build/bazel/semver"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

type ExecutorServer struct {
	grpcServer *grpc.Server
	listenAddr string
	workDir    string
	cache      interfaces.Cache
}

func New(cfg *cc.Configure) (*ExecutorServer, error) {
	executorCfg := cfg.GetExecutorConfig()
	s := &ExecutorServer{
		grpcServer: grpc.NewServer(),
		listenAddr: executorCfg.ListenAddr,
		workDir:    executorCfg.WorkDir,
		cache:      caches.GenerateCacheFromConfig(cfg.GetCacheConfig()),
	}
	debugCfg := cfg.GetDebugConfig()
	if debugCfg.LogLevel != "" {
		lev, err := logrus.ParseLevel(debugCfg.LogLevel)
		if err == nil {
			logrus.SetLevel(lev)
		} else {
			logrus.Warnf("set level to %s error: %s", debugCfg.LogLevel, err)
		}
	}
	repb.RegisterContentAddressableStorageServer(s.grpcServer, s)
	repb.RegisterExecutionServer(s.grpcServer, s)
	bytestream.RegisterByteStreamServer(s.grpcServer, s)
	repb.RegisterCapabilitiesServer(s.grpcServer, s)
	repb.RegisterActionCacheServer(s.grpcServer, s)
	return s, nil
}

func (s *ExecutorServer) Run() error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	defer lis.Close()
	logrus.Infof("baize server remote execuotor listen at addr %s", s.listenAddr)
	return s.grpcServer.Serve(lis)
}

func (s *ExecutorServer) GetCapabilities(ctx context.Context, in *repb.GetCapabilitiesRequest) (*repb.ServerCapabilities, error) {
	logrus.Tracef("registr remote execute instance %s", in.GetInstanceName())
	return &repb.ServerCapabilities{
		CacheCapabilities: &repb.CacheCapabilities{
			DigestFunctions: []repb.DigestFunction_Value{
				repb.DigestFunction_MD5,
				repb.DigestFunction_SHA1,
				repb.DigestFunction_SHA256,
				repb.DigestFunction_SHA384,
				repb.DigestFunction_SHA512,
			},
			ActionCacheUpdateCapabilities: &repb.ActionCacheUpdateCapabilities{
				UpdateEnabled: true,
			},
			// CachePriorityCapabilities: Priorities not supported.
			// MaxBatchTotalSize: Not used by Bazel yet.
			SymlinkAbsolutePathStrategy: repb.SymlinkAbsolutePathStrategy_ALLOWED,
		},
		ExecutionCapabilities: &repb.ExecutionCapabilities{
			DigestFunction: repb.DigestFunction_SHA256,
			ExecEnabled:    true,
			ExecutionPriorityCapabilities: &repb.PriorityCapabilities{
				Priorities: []*repb.PriorityCapabilities_PriorityRange{
					{MinPriority: math.MinInt32, MaxPriority: math.MaxInt32},
				},
			},
			SupportedNodeProperties: nil,
		},
		LowApiVersion:        &semver.SemVer{Major: 2},
		HighApiVersion:       &semver.SemVer{Major: 2},
		DeprecatedApiVersion: nil,
	}, nil
}
