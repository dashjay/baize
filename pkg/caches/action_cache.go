package caches

import (
	"context"
	"fmt"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
)

type ActionCache struct {
	client *redis.Client
}

func NewActionCache(options *redis.Options) *ActionCache {
	return &ActionCache{client: redis.NewClient(options)}
}

func (a *ActionCache) GetActionFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Action, error) {
	data, err := a.client.Get(ctx, digest.GetHash()).Bytes()
	if err != nil {
		return nil, err
	}
	out := &repb.Action{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}
func (a *ActionCache) GetCommandFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Command, error) {
	data, err := a.client.Get(ctx, digest.GetHash()).Bytes()
	if err != nil {
		return nil, err
	}
	out := &repb.Command{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *ActionCache) GetDirectoryFromDigest(ctx context.Context, digest *repb.Digest) (*repb.Directory, error) {
	data, err := a.client.Get(ctx, digest.GetHash()).Bytes()
	if err != nil {
		return nil, err
	}
	out := &repb.Directory{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *ActionCache) GetActionResultFromDigest(ctx context.Context, digest *repb.Digest) (*repb.ActionResult, error) {
	data, err := a.client.Get(ctx, generateActionResultKey(digest)).Bytes()
	if err != nil {
		return nil, err
	}
	out := &repb.ActionResult{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *ActionCache) PutActionResultByDigest(ctx context.Context, digest *repb.Digest, actionResult *repb.ActionResult) error {
	data, err := proto.Marshal(actionResult)
	if err != nil {
		return err
	}
	return a.client.Set(ctx, digest.GetHash(), data, -1).Err()
}

var _ interfaces.ActionCache = (*ActionCache)(nil)

func generateActionResultKey(digest *repb.Digest) string {
	return fmt.Sprintf("%s-action_result", digest.GetHash())
}
