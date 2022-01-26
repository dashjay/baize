package server

import (
	"context"
	"github.com/dashjay/bazel-remote-exec/pkg/caches"
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"math"
	"net"
	"os"

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

func New(cfg *config.Configure) (*ExecutorServer, error) {
	s := &ExecutorServer{}

	// cache
	cacheCfg := cfg.GetCacheConfig()
	updateCache := func(cache interfaces.Cache) {
		if s.cache == nil {
			s.cache = cache
		} else {
			s.cache = caches.NewComposedCache(cache, s.cache, caches.ModeReadThrough|caches.ModeWriteThrough)
		}
	}
	if cacheCfg.InmemoryCache.Enabled {
		updateCache(caches.NewMemoryCache(cacheCfg.RedisCache))
	}
	if cacheCfg.RedisCache.Enabled {
		updateCache(caches.NewRedisCache(cacheCfg.RedisCache))
	}
	if cacheCfg.DiskCache.Enabled {
		updateCache(caches.NewDiskCache(cacheCfg.DiskCache))
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

	executorCfg := cfg.GetExecutorConfig()
	s.grpcServer = grpc.NewServer()
	s.listenAddr = executorCfg.ListenAddr
	s.workDir = executorCfg.WorkDir
	_, err := os.Stat(s.workDir)
	if err == nil {
		logrus.Infof("set workdir to %s success", s.workDir)
	} else {
		err = os.MkdirAll(s.workDir, 0644)
		if err != nil {
			logrus.Panicf("set workdir to %s error: %s", s.workDir, err)
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
	logrus.SetLevel(logrus.DebugLevel)
	logrus.Infof("Listen bazel remote exec server in %s", s.listenAddr)
	return s.grpcServer.Serve(lis)
}

func (s *ExecutorServer) GetCapabilities(ctx context.Context, in *repb.GetCapabilitiesRequest) (*repb.ServerCapabilities, error) {
	logrus.Infoln("GetCapabilities: ", in.InstanceName)
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
				UpdateEnabled: false,
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
