package utils

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/utils/digest"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
)

func CalSHA256OfInput(input []byte) *repb.Digest {
	h := sha256.New()
	h.Write(input)
	return &repb.Digest{Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: int64(len(input))}
}

func CalSHA256FromReader(r io.Reader) (*repb.Digest, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return nil, err
	}
	return &repb.Digest{Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: n}, nil
}

func RandomBytes(byteLength int) []byte {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	length := len(letters)
	output := make([]byte, byteLength)
	if _, err := rand.Read(output); err != nil {
		panic(err)
	}
	// Run through output; replacing each with the equivalent random char.
	for i, b := range output {
		output[i] = letters[b%byte(length)]
	}
	return output
}

func RandomString(stringLength int) string {
	return string(RandomBytes(stringLength))
}

func WriteToBytestreamServer(ctx context.Context, cli bytestream.ByteStreamClient, d *repb.Digest,
	instanceName string, body io.Reader, opts ...grpc.CallOption) (*bytestream.WriteResponse, error) {
	wr, err := cli.Write(ctx, opts...)
	if err != nil {
		return nil, err
	}
	res := digest.NewResourceName(d, instanceName)
	uploadString, err := res.UploadString()
	if err != nil {
		return nil, err
	}
	if d.SizeBytes < cc.DefaultReadCapacity {
		var buf = make([]byte, d.SizeBytes)
		_, err = body.Read(buf)
		if err != nil {
			return nil, err
		}
		err = wr.Send(&bytestream.WriteRequest{ResourceName: uploadString, WriteOffset: 0, FinishWrite: true, Data: buf})
		if err != nil {
			return nil, err
		}
		err = wr.CloseSend()
		if err != nil {
			return nil, err
		}
	} else {
		var commited int64
		var buf = make([]byte, cc.DefaultReadCapacity)
		for commited < d.SizeBytes {
			n, err := body.Read(buf)
			if err != nil {
				return nil, err
			}
			finishWrite := commited+int64(n) >= d.SizeBytes
			err = wr.Send(&bytestream.WriteRequest{ResourceName: uploadString, WriteOffset: int64(commited), FinishWrite: finishWrite, Data: buf[:n]})
			if err != nil {
				return nil, err
			}
			commited += int64(n)
		}
		err = wr.CloseSend()
		if err != nil {
			return nil, err
		}
	}
	return wr.CloseAndRecv()
}
func ReadFromBytestreamServer(ctx context.Context, cli bytestream.ByteStreamClient, d *repb.Digest,
	instanceName string, opts ...grpc.CallOption) (io.ReadCloser, error) {

	res := digest.NewResourceName(d, instanceName)
	r, err := cli.Read(ctx, &bytestream.ReadRequest{ResourceName: res.DownloadString()}, opts...)
	if err != nil {
		return nil, err
	}
	var 
	for {
		resp, err := r.Recv()
		if err != nil {
			return 
		}
	}
}
