package baize

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils/status"

	"google.golang.org/protobuf/proto"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
)

func (s *ExecutorServer) GetActionResult(ctx context.Context, in *repb.GetActionResultRequest) (*repb.ActionResult, error) {
	acCache, err := s.cache.WithIsolation(ctx, interfaces.ActionCacheType, in.GetInstanceName())
	if err != nil {
		return nil, err
	}
	data, err := acCache.Get(ctx, in.GetActionDigest())
	if err != nil {
		return nil, err
	}
	out := &repb.ActionResult{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) UpdateActionResult(ctx context.Context, in *repb.UpdateActionResultRequest) (*repb.ActionResult, error) {
	err := s.putActionResultByDigest(ctx, in.GetActionDigest(), in.GetActionResult(), in.GetInstanceName())
	if err != nil {
		return nil, status.InternalErrorf("update action result error: %s", err)
	}
	return in.GetActionResult(), nil
}

func (s *ExecutorServer) getActionFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Action, error) {
	logrus.Tracef("invoke getActionFromDigest with %#v", digest)
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, "")
	if err != nil {
		return nil, err
	}
	data, err := casCache.Get(ctx, digest)
	if err != nil {
		return nil, err
	}
	out := &repb.Action{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) putActionResultByDigest(ctx context.Context, digest *repb.Digest, actionResult *repb.ActionResult, instanceName string) error {
	logrus.Tracef("invoke putActionResultByDigest with %#v", digest)
	data, err := proto.Marshal(actionResult)
	if err != nil {
		return status.FailedPreconditionErrorf("marshal action result error: %s", err)
	}
	acCache, err := s.cache.WithIsolation(ctx, interfaces.ActionCacheType, instanceName)
	if err != nil {
		return status.FailedPreconditionErrorf("get cache error: %s", err)
	}
	return acCache.Set(ctx, digest, data)
}
