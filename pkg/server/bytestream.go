package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dashjay/bazel-remote-exec/pkg/caches"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// bytestream.RegisterByteStreamServer(s, &RemoteExecServer{})
func (s *ExecutorServer) Read(in *bytestream.ReadRequest, server bytestream.ByteStream_ReadServer) error {
	ctx := context.Background()
	logrus.Trace("Received CAS Read request: %s", in)
	// Parse resource name per Bazel API specification
	resource, err := ParseReadResource(in.GetResourceName())
	if err != nil {
		return handleGrpcError(codes.InvalidArgument, err, "Failed to parse resource name")
	}
	// Input validation per API spec
	if in.GetReadOffset() < 0 {
		msg := fmt.Sprintf("Invalid read offset %d", in.GetReadOffset())
		logrus.WithField("readOffset", in.GetReadOffset()).Error(msg)
		return status.Error(codes.OutOfRange, msg)
	}
	if in.GetReadLimit() < 0 {
		const msg = "Read limit < 0 invalid"
		logrus.WithField("readLimit", in.GetReadLimit()).Error(msg)
		return status.Error(codes.InvalidArgument, msg)
	}
	storeName := resource.StoreName()
	data, err := s.cache.Get(ctx, resource.Digest)
	if err != nil {
		msg := fmt.Sprintf("Failed to Get %s", storeName)
		logrus.WithField("storeName", storeName).WithError(err).Errorf(msg)
		return status.Errorf(codes.Internal, msg)
	}
	chunkSize := int64(DefaultReadCapacity)
	// Set a capacity based on ReadLimit or content size
	if in.GetReadLimit() > 0 && in.GetReadLimit() < chunkSize {
		chunkSize = in.GetReadLimit()
	}
	n := int64(len(data))
	b := make([]byte, 0, chunkSize)
	var cur int64
	for ; cur < n; cur += chunkSize {
		if cur+chunkSize > n {
			b = data[cur:n]
		} else {
			b = data[cur : cur+chunkSize]
		}
		if err := server.Send(&bytestream.ReadResponse{Data: b}); err != nil {
			return handleGrpcError(codes.Internal, err, "Failed to send ReadResponse")
		}
	}
	logrus.WithFields(logrus.Fields{
		"storeName": storeName,
		"length":    cur,
	}).Trace("Finished sending data for Read")
	return nil
}

func (s *ExecutorServer) Write(stream bytestream.ByteStream_WriteServer) error {
	ctx := context.Background()
	logrus.Trace("Received CAS Write request")
	request, err := stream.Recv()
	if err != nil {
		return handleGrpcError(codes.Internal, err, "Failed to Recv()")
	}
	resource, err := ParseWriteResource(request.GetResourceName())
	if err != nil {
		return handleGrpcError(codes.InvalidArgument, err, "Parsing resource")
	}

	logrus.Tracef("Using resource name: %s", request.GetResourceName())

	// If the client is attempting to write empty/nil/size-0 data, just return as if we succeeded
	if resource.Digest.GetHash() == EmptySha {
		logrus.Infof("Request to write empty sha - bypassing Store write and Closing")
		res := &bytestream.WriteResponse{CommittedSize: EmptySize}
		return handleGrpcError(codes.Internal, stream.SendAndClose(res), "SendAndClose() for EmptySha")
	}
	p := make([]byte, 0, resource.Digest.GetSizeBytes())
	buffer := bytes.NewBuffer(p)

	storeName := resource.StoreName()
	if _, err := s.cache.Get(ctx, resource.Digest); err == nil {
		// Find it
		logrus.Infof("Resource exists in store: %s. Using client digest size: %d", storeName, resource.Digest.GetSizeBytes())
		res := &bytestream.WriteResponse{CommittedSize: resource.Digest.GetSizeBytes()}
		err = stream.SendAndClose(res)
		if err != nil {
			return handleGrpcError(codes.Internal, err, "SendAndClose() for Existing")
		}
		return nil
	} else if !caches.IsNotFoundError(err) {
		return handleGrpcError(codes.Internal, err, fmt.Sprintf("Store failed checking existence of %s", storeName))
	}

	var committed int64
	for {
		// Validate subsequent WriteRequest fields
		if request.GetWriteOffset() != committed {
			return handleGrpcError(codes.InvalidArgument, fmt.Errorf("got %d after committing %d bytes", request.GetWriteOffset(), committed), "WriteOffset invalid")
		}
		buffer.Write(request.GetData())
		committed += int64(len(request.GetData()))

		// Per API, client indicates all data has been sent
		if request.GetFinishWrite() {
			break
		}
		request, err = stream.Recv()
		if err != nil {
			return handleGrpcError(codes.Internal, err, "Failed to Recv()")
		}
	}
	// Verify committed length with Digest size
	if committed != resource.Digest.GetSizeBytes() {
		return handleGrpcError(codes.InvalidArgument, fmt.Errorf("%d mismatch with request Digest size: %d", committed, resource.Digest.GetSizeBytes()), "Data to be written len")
	}
	// Verify buffer SHA with Digest SHA
	sha := sha256.Sum256(buffer.Bytes())
	if bufferHash := hex.EncodeToString(sha[:]); bufferHash != resource.Digest.GetHash() {
		msg := "Data to be written did not hash to given Digest"
		logrus.WithFields(logrus.Fields{
			"bufferHash":   bufferHash,
			"resourceHash": resource.Digest.GetHash(),
		}).Error(msg)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	if err := s.cache.Set(ctx, resource.Digest, buffer.Bytes()); err != nil {
		return handleGrpcError(codes.Internal, err, "Store failed to Write")
	}
	if err := stream.SendAndClose(&bytestream.WriteResponse{CommittedSize: committed}); err != nil {
		return handleGrpcError(codes.Internal, err, "Error during SendAndClose()")
	}
	logrus.WithFields(logrus.Fields{
		"storeName": storeName,
		"length":    committed,
	}).Trace("Finished handling Write request")
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
		b, err = s.cache.Get(ctx, resource.Digest)
		if err != nil {
			logrus.WithField("storeName", resource.StoreName()).WithError(err).Error("Get")
			return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Not found: %s", resource.StoreName()))
		}
	}

	return &bytestream.QueryWriteStatusResponse{
		CommittedSize: int64(len(b)),
		Complete:      true,
	}, nil
}

func handleGrpcError(c codes.Code, err error, msg string) error {
	if err == nil {
		return nil
	}
	logrus.WithError(err).Errorf(msg)
	return status.Error(c, err.Error())
}
