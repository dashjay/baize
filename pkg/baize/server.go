package baize

import (
	"context"
	"math"
	"net"
	"net/http"

	"github.com/dashjay/baize/pkg/cache_server"
	"github.com/dashjay/baize/pkg/caches"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/scheduler"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/bazelbuild/remote-apis/build/bazel/semver"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

type Server struct {
	schedulerServer *scheduler.Scheduler
	cacheServer     *cache_server.Server
	grpcServer      *grpc.Server
	cfg             *cc.Configure
}

func New(cfg *cc.Configure) (*Server, error) {
	cache := caches.GenerateCacheFromConfig(cfg.GetCacheConfig())
	s := &Server{
		schedulerServer: scheduler.NewScheduler(cache),
		cacheServer:     cache_server.New(cache),
		cfg:             cfg,
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
	repb.RegisterContentAddressableStorageServer(s.grpcServer, s.cacheServer)
	repb.RegisterExecutionServer(s.grpcServer, s.schedulerServer)
	bytestream.RegisterByteStreamServer(s.grpcServer, s.cacheServer)
	repb.RegisterCapabilitiesServer(s.grpcServer, s)
	repb.RegisterActionCacheServer(s.grpcServer, s.cacheServer)
	return s, nil
}

func (s *Server) Run() error {
	lis, err := net.Listen("tcp", s.cfg.ServerConfig.ListenAddr)
	if err != nil {
		return err
	}
	if pprofAddr := s.cfg.GetExecutorConfig().PprofAddr; pprofAddr != "" {
		go func() {
			http.ListenAndServe(pprofAddr, nil)
		}()
	}
	defer lis.Close()
	logrus.Infof("baize server remote execuotor listen at addr %s", s.cfg.ServerConfig.ListenAddr)
	return s.grpcServer.Serve(lis)
}

func (s *Server) GetCapabilities(ctx context.Context, in *repb.GetCapabilitiesRequest) (*repb.ServerCapabilities, error) {
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
