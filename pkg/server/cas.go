package server

import (
	"context"
	"github.com/dashjay/bazel-remote-exec/pkg/caches"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/sirupsen/logrus"
)

func (s *ExecutorServer) FindMissingBlobs(ctx context.Context, in *repb.FindMissingBlobsRequest) (*repb.FindMissingBlobsResponse, error) {
	logrus.Debugf("Received CAS FindMissingBlobs request, InstanceName: %s, Blobs size: %d", in.GetInstanceName(), len(in.GetBlobDigests()))
	ret := &repb.FindMissingBlobsResponse{
		MissingBlobDigests: []*repb.Digest{},
	}
	for _, digest := range in.GetBlobDigests() {
		if _, err := s.cache.Get(ctx, digest); err != nil {
			if caches.IsNotFoundError(err) {
				ret.MissingBlobDigests = append(ret.MissingBlobDigests, digest)
			} else {
				logrus.WithError(err).Error("s.store.Get")
				return nil, err
			}
		}
	}
	return ret, nil
}
func (s *ExecutorServer) BatchUpdateBlobs(ctx context.Context, in *repb.BatchUpdateBlobsRequest) (*repb.BatchUpdateBlobsResponse, error) {
	err := status.Error(codes.Unimplemented, "This service does not support BatchUpdateBlobs")
	logrus.WithError(err).Error("Unimplemented")
	return nil, err
}
func (s *ExecutorServer) BatchReadBlobs(ctx context.Context, in *repb.BatchReadBlobsRequest) (*repb.BatchReadBlobsResponse, error) {
	err := status.Error(codes.Unimplemented, "This service does not support BatchReadBlobs")
	logrus.WithError(err).Error("Unimplemented")
	return nil, err
}
func (s *ExecutorServer) GetTree(in *repb.GetTreeRequest, server repb.ContentAddressableStorage_GetTreeServer) error {
	err := status.Error(codes.Unimplemented, "This service does not support downloading directory trees")
	logrus.WithError(err).Error("Unimplemented")
	return err
}
