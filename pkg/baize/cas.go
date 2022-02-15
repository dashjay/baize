package baize

import (
	"context"

	"github.com/dashjay/baize/pkg/interfaces"

	nstatus "google.golang.org/genproto/googleapis/rpc/status"

	"github.com/dashjay/baize/pkg/utils/status"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/sirupsen/logrus"
)

func (s *ExecutorServer) FindMissingBlobs(ctx context.Context, in *repb.FindMissingBlobsRequest) (*repb.FindMissingBlobsResponse, error) {
	ret := &repb.FindMissingBlobsResponse{
		MissingBlobDigests: []*repb.Digest{},
	}
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, in.GetInstanceName())
	if err != nil {
		return nil, err
	}
	for _, digest := range in.GetBlobDigests() {
		if _, err := casCache.Get(ctx, digest); err != nil {
			if status.IsNotFoundError(err) {
				ret.MissingBlobDigests = append(ret.MissingBlobDigests, digest)
			} else {
				logrus.WithError(err).Error("s.store.Get")
				return nil, err
			}
		}
	}
	logrus.Debugf("Received CAS FindMissingBlobs request, InstanceName: %s, Blobs size: %d, Misssing Item Nums: %d", in.GetInstanceName(), len(in.GetBlobDigests()), len(ret.MissingBlobDigests))
	return ret, nil
}
func (s *ExecutorServer) BatchUpdateBlobs(ctx context.Context, in *repb.BatchUpdateBlobsRequest) (*repb.BatchUpdateBlobsResponse, error) {
	logrus.Tracef("invoke BatchUpdateBlobs with %#v", in)
	resp := &repb.BatchUpdateBlobsResponse{}
	for i := range in.Requests {
		d := in.Requests[i].GetDigest()
		wr, err := s.cache.Writer(ctx, d)
		if err != nil {
			return resp, err
		}
		_, err = wr.Write(in.Requests[i].GetData())
		wr.Close()
		if err == nil {
			resp.Responses = append(resp.Responses, &repb.BatchUpdateBlobsResponse_Response{Digest: d, Status: &nstatus.Status{Code: 0, Message: "success"}})
		} else {
			resp.Responses = append(resp.Responses, &repb.BatchUpdateBlobsResponse_Response{Digest: d, Status: &nstatus.Status{Code: 1, Message: "success"}})
		}
	}
	return resp, nil
}
func (s *ExecutorServer) BatchReadBlobs(ctx context.Context, in *repb.BatchReadBlobsRequest) (*repb.BatchReadBlobsResponse, error) {
	digests := in.GetDigests()
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, in.GetInstanceName())
	if err != nil {
		return nil, err
	}

	var responses []*repb.BatchReadBlobsResponse_Response
	for k := range digests {
		outs, err := casCache.Get(ctx, digests[k])
		resp := &repb.BatchReadBlobsResponse_Response{
			Digest: digests[k],
			Data:   outs,
		}
		if err != nil {
			resp.Status = &nstatus.Status{Code: 1, Message: err.Error()}
		} else {
			resp.Status = &nstatus.Status{Code: 0, Message: err.Error()}
		}
		responses = append(responses, resp)
	}
	return &repb.BatchReadBlobsResponse{Responses: responses}, nil
}
func (s *ExecutorServer) GetTree(in *repb.GetTreeRequest, server repb.ContentAddressableStorage_GetTreeServer) error {
	logrus.Tracef("invoke GetTree with %#v", in)
	err := status.UnimplementedError("This service does not support GetTree")
	logrus.WithError(err).Error("Unimplemented")
	return err
}
