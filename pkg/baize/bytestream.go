package baize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"

	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils/status"
)

// bytestream.RegisterByteStreamServer(s, &RemoteExecServer{})
func (s *ExecutorServer) Read(in *bytestream.ReadRequest, server bytestream.ByteStream_ReadServer) error {
	logrus.Tracef("invoke read from %s", in.GetResourceName())
	ctx := context.Background()
	// Parse resource name per Bazel API specification
	resource, err := ParseReadResource(in.GetResourceName())
	if err != nil {
		return status.InvalidArgumentErrorf("failed to parse resource name: [%s]", in.GetResourceName())
	}
	// Input validation per API spec
	if in.GetReadOffset() < 0 {
		msg := fmt.Sprintf("Invalid read offset %d", in.GetReadOffset())
		logrus.WithField("readOffset", in.GetReadOffset()).Error(msg)
		return status.OutOfRangeErrorf("read offset <0")
	}
	if in.GetReadLimit() < 0 {
		msg := "Read limit < 0 invalid"
		logrus.WithField("readLimit", in.GetReadLimit()).Error(msg)
		return status.OutOfRangeError(msg)
	}
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, resource.Instance)
	if err != nil {
		return status.InternalErrorf("get cache error: %s", err)
	}
	rd, err := casCache.Reader(ctx, resource.Digest, 0)
	if err != nil {
		return status.NotFoundErrorf("key %s not found", resource.Digest)
	}
	chunkSize := int64(DefaultReadCapacity)
	// Set a capacity based on ReadLimit or content size
	if in.GetReadLimit() > 0 && in.GetReadLimit() < chunkSize {
		chunkSize = in.GetReadLimit()
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

func (s *ExecutorServer) Write(stream bytestream.ByteStream_WriteServer) error {
	ctx := context.Background()
	request, err := stream.Recv()
	if err != nil {
		return status.InternalErrorf("fail to call stream.Recv(): %s", err)
	}
	resource, err := ParseWriteResource(request.GetResourceName())
	if err != nil {
		return status.InvalidArgumentErrorf("failed to parse resource name: [%s]", request.GetResourceName())
	}

	logrus.Tracef("invoke write %s", request.GetResourceName())

	// If the client is attempting to write empty/nil/size-0 data, just return as if we succeeded
	if resource.Digest.GetHash() == EmptySha {
		logrus.Infof("Request to write empty sha - bypassing Store write and Closing")
		res := &bytestream.WriteResponse{CommittedSize: EmptySize}
		err = stream.SendAndClose(res)
		if err != nil {
			return status.InternalErrorf("SendAndClose() for EmptySha, error: %s", err)
		}
		return nil
	}

	if _, err := s.cache.Get(ctx, resource.Digest); err == nil {
		res := &bytestream.WriteResponse{CommittedSize: resource.Digest.GetSizeBytes()}
		err = stream.SendAndClose(res)
		if err != nil {
			return status.InternalErrorf("SendAndClose() for existing error: %s", err)
		}
		return nil
	} else if !status.IsNotFoundError(err) {
		return status.InternalErrorf("Store failed checking existence of %s", resource.Digest.GetHash())
	}

	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, resource.Instance)
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
		request, err = stream.Recv()
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
	if err := stream.SendAndClose(&bytestream.WriteResponse{CommittedSize: committed}); err != nil {
		return status.InternalErrorf("Error during SendAndClose(): %s", err)
	}
	return nil
}
func (s *ExecutorServer) QueryWriteStatus(ctx context.Context, in *bytestream.QueryWriteStatusRequest) (*bytestream.QueryWriteStatusResponse, error) {
	resource, err := ParseWriteResource(in.GetResourceName())
	if err != nil {
		logrus.WithError(err).Error("parseResourceNameWrite")
		return nil, err
	}
	var b []byte
	if resource.Digest.GetHash() != EmptySha {
		casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, resource.Instance)
		if err != nil {
			return nil, status.InternalErrorf("get cache error: %s", err)
		}
		b, err = casCache.Get(ctx, resource.Digest)
		if err != nil {
			return nil, status.NotFoundErrorf("Not found: %s", resource.StoreName())
		}
	}
	return &bytestream.QueryWriteStatusResponse{
		CommittedSize: int64(len(b)),
		Complete:      true,
	}, nil
}
