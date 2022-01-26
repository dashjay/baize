package server

import (
	"context"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExecutorServer) GetActionResult(ctx context.Context, in *repb.GetActionResultRequest) (*repb.ActionResult, error) {
	result, err := s.GetActionResultFromDigest(ctx, in.GetActionDigest())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, err.Error())
	}
	return result, nil
}

func (s *ExecutorServer) UpdateActionResult(ctx context.Context, in *repb.UpdateActionResultRequest) (*repb.ActionResult, error) {
	return nil, nil
}

func (s *ExecutorServer) GetActionFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Action, error) {
	data, err := s.cache.Get(ctx, digest)
	if err != nil {
		return nil, err
	}
	out := &repb.Action{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *ExecutorServer) GetCommandFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Command, error) {
	data, err := s.cache.Get(ctx, digest)
	if err != nil {
		return nil, err
	}
	out := &repb.Command{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) GetDirectoryFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Directory, error) {
	data, err := s.cache.Get(ctx, digest)
	if err != nil {
		return nil, err
	}
	out := &repb.Directory{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) GetActionResultFromDigest(ctx context.Context, digest *repb.Digest) (*repb.ActionResult, error) {
	data, err := s.cache.Get(ctx, digest)
	if err != nil {
		return nil, err
	}
	out := &repb.ActionResult{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) PutActionResultByDigest(ctx context.Context, digest *repb.Digest, actionResult *repb.ActionResult) error {
	data, err := proto.Marshal(actionResult)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, digest, data)
}
