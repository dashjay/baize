package scheduler

import (
	"context"
	"fmt"
	"sync"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils/digest"
	"github.com/dashjay/baize/pkg/utils/status"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/longrunning"
	googlestatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type StreamLike interface {
	Context() context.Context
	Send(*longrunning.Operation) error
}

type StateChangeFunc func(stage repb.ExecutionStage_Value, execResponse *repb.ExecuteResponse) error

type waitOpts struct {
	isExecuteRequest bool
}

func marshalAny(pb proto.Message) (*any.Any, error) {
	pbAny, err := anypb.New(pb)
	if err != nil {
		logrus.WithError(err).Error(fmt.Sprintf("Failed to marshal proto message %q as Any: %s", pb, err))
		return nil, err
	}
	return pbAny, nil
}

func InProgressExecuteResponse() *repb.ExecuteResponse {
	return ExecuteResponseWithResult(nil, codes.OK)
}

func ExecuteResponseWithResult(ar *repb.ActionResult, code codes.Code) *repb.ExecuteResponse {
	rsp := &repb.ExecuteResponse{
		Status: &googlestatus.Status{Code: int32(code)},
	}
	if ar != nil {
		rsp.Result = ar
	}
	return rsp
}

func GetStateChangeFunc(stream StreamLike, taskID string, adInstanceDigest *digest.ResourceName) StateChangeFunc {
	return func(stage repb.ExecutionStage_Value, execResponse *repb.ExecuteResponse) error {
		op, err := Assemble(stage, taskID, adInstanceDigest, execResponse)
		if err != nil {
			return status.InternalErrorf("Error updating state of %q: %s", taskID, err)
		}

		select {
		case <-stream.Context().Done():
			logrus.Warningf("Attempted state change on %q but context is done.", taskID)
			return status.UnavailableErrorf("Context canceled: %s", stream.Context().Err())
		default:
			return stream.Send(op)
		}
	}
}

func Assemble(stage repb.ExecutionStage_Value, name string, r *digest.ResourceName, er *repb.ExecuteResponse) (*longrunning.Operation, error) {
	if r == nil || er == nil {
		return nil, status.FailedPreconditionError("digest or execute response are both required to assemble operation")
	}

	metadata, err := anypb.New(&repb.ExecuteOperationMetadata{
		Stage:        stage,
		ActionDigest: r.GetDigest(),
	})
	if err != nil {
		return nil, err
	}
	operation := &longrunning.Operation{
		Name:     name,
		Metadata: metadata,
	}
	result, err := anypb.New(er)
	if err != nil {
		return nil, err
	}
	operation.Result = &longrunning.Operation_Response{Response: result}

	if stage == repb.ExecutionStage_COMPLETED {
		operation.Done = true
	}
	return operation, nil
}

func ValidateActionResult(ctx context.Context, cache interfaces.Cache, r *repb.ActionResult) error {
	outputFileDigests := make([]*repb.Digest, 0, len(r.OutputFiles))
	mu := &sync.Mutex{}
	appendDigest := func(d *repb.Digest) {
		if d != nil && d.GetSizeBytes() > 0 {
			mu.Lock()
			outputFileDigests = append(outputFileDigests, d)
			mu.Unlock()
		}
	}
	for _, f := range r.OutputFiles {
		appendDigest(f.GetDigest())
	}

	for i := range r.OutputDirectories {
		blob, err := cache.Get(ctx, r.OutputDirectories[i].GetTreeDigest())
		if err != nil {
			logrus.WithError(err).Errorf("get %s while validating action reseult error", r.OutputDirectories[i].GetTreeDigest())
			return err
		}
		tree := &repb.Tree{}
		if err := proto.Unmarshal(blob, tree); err != nil {
			logrus.WithError(err).Errorf("unmarshal data of %s while validating action reseult error", r.OutputDirectories[i].GetTreeDigest())
			return err
		}
		for _, f := range tree.GetRoot().GetFiles() {
			appendDigest(f.GetDigest())
		}
		for _, dir := range tree.GetChildren() {
			for _, f := range dir.GetFiles() {
				appendDigest(f.GetDigest())
			}
		}
	}

	return checkFilesExist(ctx, cache, outputFileDigests)
}

func CASCache(ctx context.Context, cache interfaces.Cache, instanceName string) (interfaces.Cache, error) {
	return cache.WithIsolation(ctx, interfaces.CASCacheType, instanceName)
}

func ActionCache(ctx context.Context, cache interfaces.Cache, instanceName string) (interfaces.Cache, error) {
	return cache.WithIsolation(ctx, interfaces.ActionCacheType, instanceName)
}

func checkFilesExist(ctx context.Context, cache interfaces.Cache, digests []*repb.Digest) error {
	missing, err := cache.FindMissing(ctx, digests)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return status.NotFoundErrorf("ActionResult output file: '%s' not found in cache", missing[0])
	}
	return nil
}
