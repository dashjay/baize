package cache_server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils/status"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"
	"io"
)

type Server struct {
	cache interfaces.Cache
}

func (c *Server) GetActionResult(ctx context.Context, request *repb.GetActionResultRequest) (*repb.ActionResult, error) {
	acCache, err := c.cache.WithIsolation(ctx, interfaces.ActionCacheType, request.GetInstanceName())
	if err != nil {
		return nil, err
	}
	data, err := acCache.Get(ctx, request.GetActionDigest())
	if err != nil {
		return nil, err
	}
	out := &repb.ActionResult{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Server) UpdateActionResult(ctx context.Context, request *repb.UpdateActionResultRequest) (*repb.ActionResult, error) {
	logrus.Tracef("invoke UpdateActionResult with %s", request.GetActionDigest())
	data, err := proto.Marshal(request.ActionResult)
	if err != nil {
		return request.ActionResult, status.FailedPreconditionErrorf("marshal action result error: %s", err)
	}
	acCache, err := c.cache.WithIsolation(ctx, interfaces.ActionCacheType, request.InstanceName)
	if err != nil {
		return request.ActionResult, status.FailedPreconditionErrorf("get cache error: %s", err)
	}
	return request.ActionResult, acCache.Set(ctx, request.GetActionDigest(), data)
}

func (c *Server) Read(request *bytestream.ReadRequest, server bytestream.ByteStream_ReadServer) error {
	logrus.Tracef("invoke read from %s", request.GetResourceName())
	ctx := context.Background()
	// Parse resource name per Bazel API specification
	resource, err := ParseReadResource(request.GetResourceName())
	if err != nil {
		return status.InvalidArgumentErrorf("failed to parse resource name: [%s]", request.GetResourceName())
	}
	// Input validation per API spec
	if request.GetReadOffset() < 0 {
		msg := fmt.Sprintf("Invalid read offset %d", request.GetReadOffset())
		logrus.WithField("readOffset", request.GetReadOffset()).Error(msg)
		return status.OutOfRangeErrorf("read offset <0")
	}
	if request.GetReadLimit() < 0 {
		msg := "Read limit < 0 invalid"
		logrus.WithField("readLimit", request.GetReadLimit()).Error(msg)
		return status.OutOfRangeError(msg)
	}
	casCache, err := c.cache.WithIsolation(ctx, interfaces.CASCacheType, resource.Instance)
	if err != nil {
		return status.InternalErrorf("get cache error: %s", err)
	}
	rd, err := casCache.Reader(ctx, resource.Digest, 0)
	if err != nil {
		return status.NotFoundErrorf("key %s not found", resource.Digest)
	}
	chunkSize := int64(cc.DefaultReadCapacity)
	// Set a capacity based on ReadLimit or content size
	if request.GetReadLimit() > 0 && request.GetReadLimit() < chunkSize {
		chunkSize = request.GetReadLimit()
	}
	var finish = false
	var b = make([]byte, chunkSize)
	for !finish {
		n, err := io.LimitReader(rd, chunkSize).Read(b)
		if err != nil {
			if err != io.EOF {
				return status.InternalErrorf("write section to client error: %s", err)
			}
			finish = true
		}
		if n != 0 {
			if err := server.Send(&bytestream.ReadResponse{Data: b[:n]}); err != nil {
				return status.InternalErrorf("fail to send response to client: %s", err)
			}
		}
	}
	return nil
}

func (c *Server) Write(server bytestream.ByteStream_WriteServer) error {
	ctx := context.Background()
	request, err := server.Recv()
	if err != nil {
		return status.InternalErrorf("fail to call stream.Recv(): %s", err)
	}
	resource, err := ParseWriteResource(request.GetResourceName())
	if err != nil {
		return status.InvalidArgumentErrorf("failed to parse resource name: [%s]", request.GetResourceName())
	}

	logrus.Tracef("invoke write %s", request.GetResourceName())

	// If the client is attempting to write empty/nil/size-0 data, just return as if we succeeded
	if resource.Digest.GetHash() == cc.EmptySha {
		logrus.Infof("Request to write empty sha - bypassing Store write and Closing")
		res := &bytestream.WriteResponse{CommittedSize: cc.EmptySize}
		err = server.SendAndClose(res)
		if err != nil {
			return status.InternalErrorf("SendAndClose() for EmptySha, error: %s", err)
		}
		return nil
	}

	if _, err := c.cache.Get(ctx, resource.Digest); err == nil {
		res := &bytestream.WriteResponse{CommittedSize: resource.Digest.GetSizeBytes()}
		err = server.SendAndClose(res)
		if err != nil {
			return status.InternalErrorf("SendAndClose() for existing error: %s", err)
		}
		return nil
	} else if !status.IsNotFoundError(err) {
		return status.InternalErrorf("Store failed checking existence of %s", resource.Digest.GetHash())
	}

	casCache, err := c.cache.WithIsolation(ctx, interfaces.CASCacheType, resource.Instance)
	if err != nil {
		return status.InternalErrorf("get cache error: %s", err)
	}
	wc, err := casCache.Writer(ctx, resource.Digest)
	if err != nil {
		return status.InternalErrorf("get writer error: %s", err)
	}
	defer wc.Close()
	h := sha256.New()
	var committed int64
	mw := io.MultiWriter(wc, h)
	for {
		// Validate subsequent WriteRequest fields
		if request.GetWriteOffset() != committed {
			return status.InvalidArgumentErrorf("got %d after committing %d bytes", request.GetWriteOffset(), committed)
		}
		n, err := mw.Write(request.GetData())
		if err != nil {
			return status.InternalErrorf("write data into cache error: %s", err)
		}
		committed += int64(n)

		// Per API, client indicates all data has been sent
		if request.GetFinishWrite() {
			break
		}
		request, err = server.Recv()
		if err != nil {
			return status.InternalErrorf("Failed to Recv(): %s", err)
		}
	}
	// Verify committed length with Digest size
	if committed != resource.Digest.GetSizeBytes() {
		return status.InvalidArgumentErrorf("%d mismatch with request Digest size: %d", committed, resource.Digest.GetSizeBytes())
	}

	if bufferHash := hex.EncodeToString(h.Sum(nil)); bufferHash != resource.Digest.GetHash() {
		msg := "Data to be written did not hash to given Digest"
		logrus.WithFields(logrus.Fields{
			"bufferHash":   bufferHash,
			"resourceHash": resource.Digest.GetHash(),
		}).Error(msg)
		return status.InvalidArgumentError(msg)
	}
	if err := server.SendAndClose(&bytestream.WriteResponse{CommittedSize: committed}); err != nil {
		return status.InternalErrorf("Error during SendAndClose(): %s", err)
	}
	return nil
}

func (c *Server) QueryWriteStatus(ctx context.Context, request *bytestream.QueryWriteStatusRequest) (*bytestream.QueryWriteStatusResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c *Server) FindMissingBlobs(ctx context.Context, request *repb.FindMissingBlobsRequest) (*repb.FindMissingBlobsResponse, error) {
	ret := &repb.FindMissingBlobsResponse{
		MissingBlobDigests: []*repb.Digest{},
	}
	casCache, err := c.cache.WithIsolation(ctx, interfaces.CASCacheType, request.GetInstanceName())
	if err != nil {
		return nil, err
	}
	for _, digest := range request.GetBlobDigests() {
		if exists, err := casCache.Contains(ctx, digest); err != nil && exists {
			if status.IsNotFoundError(err) {
				ret.MissingBlobDigests = append(ret.MissingBlobDigests, digest)
			} else {
				logrus.WithError(err).WithField("digest", digest.Hash).Errorln("find missing blobs error")
				return nil, err
			}
		}
	}
	logrus.Debugf("Received CAS FindMissingBlobs request, InstanceName: %s, Blobs size: %d, Misssing Item Nums: %d",
		request.GetInstanceName(), len(request.GetBlobDigests()), len(ret.MissingBlobDigests))
	return ret, nil
}

func (c *Server) BatchUpdateBlobs(ctx context.Context, request *repb.BatchUpdateBlobsRequest) (*repb.BatchUpdateBlobsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c *Server) BatchReadBlobs(ctx context.Context, request *repb.BatchReadBlobsRequest) (*repb.BatchReadBlobsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c *Server) GetTree(request *repb.GetTreeRequest, server repb.ContentAddressableStorage_GetTreeServer) error {
	//TODO implement me
	panic("implement me")
}

var _ repb.ContentAddressableStorageServer = (*Server)(nil)
var _ bytestream.ByteStreamServer = (*Server)(nil)
var _ repb.ActionCacheServer = (*Server)(nil)
