package baize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/types/known/anypb"

	"google.golang.org/grpc/codes"

	"github.com/dashjay/baize/pkg/utils/digest"

	"google.golang.org/protobuf/types/known/timestamppb"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/longrunning"

	googlestatus "google.golang.org/genproto/googleapis/rpc/status"

	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/utils"
	"github.com/dashjay/baize/pkg/utils/commandutil"
	"github.com/dashjay/baize/pkg/utils/status"
)

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

	var wg sync.WaitGroup

	for idx := range r.OutputDirectories {
		wg.Add(1)
		go func(i int) {
			blob, err := cache.Get(ctx, r.OutputDirectories[i].GetTreeDigest())
			if err != nil {
				logrus.WithError(err).Errorf("get %s while validating action reseult error", r.OutputDirectories[i].GetTreeDigest())
				return
			}
			tree := &repb.Tree{}
			if err := proto.Unmarshal(blob, tree); err != nil {
				logrus.WithError(err).Errorf("unmarshal data of %s while validating action reseult error", r.OutputDirectories[i].GetTreeDigest())
				return
			}
			for _, f := range tree.GetRoot().GetFiles() {
				appendDigest(f.GetDigest())
			}
			for _, dir := range tree.GetChildren() {
				for _, f := range dir.GetFiles() {
					appendDigest(f.GetDigest())
				}
			}
		}(idx)
	}
	wg.Wait()
	return checkFilesExist(ctx, cache, outputFileDigests)
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

type StreamLike interface {
	Context() context.Context
	Send(*longrunning.Operation) error
}

type StateChangeFunc func(stage repb.ExecutionStage_Value, execResponse *repb.ExecuteResponse) error

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
func ExecuteResponseWithResult(ar *repb.ActionResult, code codes.Code) *repb.ExecuteResponse {
	rsp := &repb.ExecuteResponse{
		Status: &googlestatus.Status{Code: int32(code)},
	}
	if ar != nil {
		rsp.Result = ar
	}
	return rsp
}
func readProtoFromCache(ctx context.Context, cache interfaces.Cache, r *digest.ResourceName, out proto.Message) error {
	data, err := cache.Get(ctx, r.GetDigest())
	if err != nil {
		if status.IsNotFoundError(err) {
			return err
		}
		return err
	}
	return proto.Unmarshal(data, out)
}

func ReadProtoFromCAS(ctx context.Context, cache interfaces.Cache, d *digest.ResourceName, out proto.Message) error {
	cas, err := CASCache(ctx, cache, d.GetInstanceName())
	if err != nil {
		return err
	}
	return readProtoFromCache(ctx, cas, d, out)
}

func CASCache(ctx context.Context, cache interfaces.Cache, instanceName string) (interfaces.Cache, error) {
	return cache.WithIsolation(ctx, interfaces.CASCacheType, instanceName)
}

func ActionCache(ctx context.Context, cache interfaces.Cache, instanceName string) (interfaces.Cache, error) {
	return cache.WithIsolation(ctx, interfaces.ActionCacheType, instanceName)
}

func (s *ExecutorServer) Execute(req *repb.ExecuteRequest, stream repb.Execution_ExecuteServer) error {
	logrus.Tracef("invoke Execute with %#v", req)

	// construct resources name
	adInstanceDigest := digest.NewResourceName(req.GetActionDigest(), req.GetInstanceName())

	// generate execution id
	executionID, err := adInstanceDigest.UploadString()
	if err != nil {
		return err
	}

	// try lookup the result from cache(AC)
	if !req.GetSkipCacheLookup() {
		acCache, err := s.cache.WithIsolation(stream.Context(), interfaces.ActionCacheType, req.GetInstanceName())
		if err != nil {
			return err
		}
		// try get from ac
		// if got, return directly
		data, err := acCache.Get(stream.Context(), req.GetActionDigest())
		if err == nil {
			actionResult := &repb.ActionResult{}
			if err := proto.Unmarshal(data, actionResult); err != nil {
				return err
			}
			casCache, err := s.cache.WithIsolation(stream.Context(), interfaces.CASCacheType, req.GetInstanceName())
			if err != nil {
				return err
			}
			err = ValidateActionResult(stream.Context(), casCache, actionResult)
			if err != nil {
				return err
			}
			stateChangeFn := GetStateChangeFunc(stream, executionID, adInstanceDigest)
			if err := stateChangeFn(repb.ExecutionStage_COMPLETED, ExecuteResponseWithResult(actionResult, codes.OK)); err != nil {
				return err
			}
			return nil
		}
	}

	// wait req
	waitReq := &repb.WaitExecutionRequest{Name: executionID}
	return s.waitExecution(waitReq, stream, waitOpts{isExecuteRequest: true})
}

type waitOpts struct {
	isExecuteRequest bool
}

func InProgressExecuteResponse() *repb.ExecuteResponse {
	return ExecuteResponseWithResult(nil, codes.OK)
}
func (s *ExecutorServer) waitExecution(req *repb.WaitExecutionRequest, stream repb.Execution_WaitExecutionServer, opts waitOpts) error {
	logrus.Tracef("invoke waitExecution with %#v", req)
	ctx := stream.Context()

	r, err := digest.ParseUploadResourceName(req.GetName())
	if err != nil {
		logrus.Errorf("could not extract digest from %q: %s", req.GetName(), err)
		return err
	}
	if opts.isExecuteRequest {
		stateChangeFn := GetStateChangeFunc(stream, req.GetName(), r)
		err = stateChangeFn(repb.ExecutionStage_UNKNOWN, InProgressExecuteResponse())
		if err != nil && err != io.EOF {
			logrus.Warningf("Could not send initial update: %s", err)
		}
	}
	action, err := s.getActionFromDigest(ctx, r.GetDigest())
	if err != nil {
		return err
	}
	// Response
	eom := &repb.ExecuteOperationMetadata{
		Stage:            repb.ExecutionStage_EXECUTING,
		ActionDigest:     r.GetDigest(),
		StdoutStreamName: "",
		StderrStreamName: "",
	}
	eomAsPBAny, err := marshalAny(eom)
	if err != nil {
		return status.InternalErrorf("failed to marshal eom: %s", err)
	}
	op := &longrunning.Operation{
		Name:     req.GetName(),
		Metadata: eomAsPBAny,
		Done:     false,
	}
	if err := stream.Send(op); err != nil {
		return err
	}
	actionResult, err := s.runWorker(ctx, action, s.workDir)
	if err != nil {
		logrus.WithError(err).Errorf("runWorker")
		return err
	}
	if err := s.putActionResultByDigest(ctx, r.GetDigest(), actionResult, r.GetInstanceName()); err != nil {
		logrus.WithError(err).Errorf("putActionResultByDigest")
		return err
	}
	response, err := marshalAny(&repb.ExecuteResponse{
		Result:  actionResult,
		Message: "action executing",
	})
	if err != nil {
		logrus.WithError(err).Errorf("marshalAny executeResponse")
		return err
	}

	eom.Stage = repb.ExecutionStage_COMPLETED
	op.Result = &longrunning.Operation_Response{Response: response}
	op.Done = true

	return stream.Send(op)
}

func (s *ExecutorServer) WaitExecution(req *repb.WaitExecutionRequest, server repb.Execution_WaitExecutionServer) error {
	return s.waitExecution(req, server, waitOpts{false})
}

func (s *ExecutorServer) getDirectoryFromDigest(ctx context.Context, d *repb.Digest) (*repb.Directory, error) {
	logrus.Tracef("invoke getDirectoryFromDigest with %s", d.GetHash())
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, "")
	if err != nil {
		return nil, err
	}
	data, err := casCache.Get(ctx, d)
	if err != nil {
		return nil, err
	}
	out := &repb.Directory{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) ensureFiles(ctx context.Context, rootDigest *repb.Digest, base string) error {
	rootDir, err := s.getDirectoryFromDigest(ctx, rootDigest)
	if err != nil {
		logrus.WithError(err).Errorf("GetDirectoryFromDigest with digest %s", rootDigest.GetHash())
		return err
	}
	if rootDir.GetFiles() == nil && rootDir.GetNodeProperties() == nil && rootDir.GetDirectories() == nil && rootDir.GetSymlinks() == nil {
		return nil
	}
	if err := s.writeFiles(ctx, rootDir.GetFiles(), base); err != nil {
		return err
	}
	for _, dir := range rootDir.GetDirectories() {
		if err := s.ensureFiles(ctx, dir.GetDigest(), filepath.Join(base, dir.GetName())); err != nil {
			return err
		}
	}
	return nil
}

func (s *ExecutorServer) writeFiles(ctx context.Context, files []*repb.FileNode, base string) error {
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, "")
	if err != nil {
		return err
	}
	for _, file := range files {
		fn := filepath.Join(base, file.GetName())
		if _, err := os.Stat(fn); err != nil && !os.IsNotExist(err) {
			return err
		} else if err == nil {
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fn), os.ModePerm); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if file.GetIsExecutable() {
			mode = os.FileMode(0555)
		}
		fi, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE, mode)
		if err != nil {
			return err
		}
		d := file.GetDigest()
		// Get file contents
		if d.GetHash() != EmptySha {
			r, err := casCache.Reader(ctx, d, 0)
			if err != nil {
				logrus.WithError(err).Error("writeFiles")
				return err
			}
			io.Copy(fi, r)
			r.Close()
		}
		fi.Close()
	}
	return nil
}

func (s *ExecutorServer) getCommandFromDigest(ctx context.Context, d *repb.Digest) (*repb.Command, error) {
	logrus.Tracef("invoke GetCommandFromDigest with %#v", d)
	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, "")
	if err != nil {
		return nil, err
	}
	data, err := casCache.Get(ctx, d)
	if err != nil {
		return nil, err
	}
	out := &repb.Command{}
	if err := proto.Unmarshal(data, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *ExecutorServer) runWorker(ctx context.Context, action *repb.Action, workdir string) (*repb.ActionResult, error) {
	if err := s.ensureFiles(ctx, action.GetInputRootDigest(), workdir); err != nil {
		logrus.WithError(err).Errorf("ensureFiles")
		return nil, err
	}

	casCache, err := s.cache.WithIsolation(ctx, interfaces.CASCacheType, "")
	if err != nil {
		return nil, err
	}

	// repb.Command
	command, err := s.getCommandFromDigest(ctx, action.GetCommandDigest())
	if err != nil {
		logrus.WithError(err).Errorf("GetCommandFromDigest")
		return nil, err
	}
	envStr := "export "
	for _, env := range command.GetEnvironmentVariables() {
		envStr += fmt.Sprintf("%s=%s ", env.Name, env.Value)
	}
	logrus.Debugln("platform: ", command.GetPlatform())
	logrus.Debugln("arguments: ", command.GetArguments())
	logrus.Debugln("environmentVariables: ", envStr)
	logrus.Debugln("outputDirectories: ", command.GetOutputDirectories())
	logrus.Debugln("outputFiles: ", command.GetOutputFiles())
	logrus.Debugln("outputNodeProperties: ", command.GetOutputNodeProperties())
	logrus.Debugln("outputPaths: ", command.GetOutputPaths())
	logrus.Debugln("workingDirectory: ", command.GetWorkingDirectory())

	// mkdir all GetOutputFiles's dir
	for _, file := range command.GetOutputFiles() {
		base := filepath.Join(workdir, filepath.Dir(file))
		if err := os.MkdirAll(base, os.ModePerm); err != nil {
			logrus.WithError(err).Errorf("os.MkdirAll")
			return nil, err
		}
	}

	var stdout bytes.Buffer
	result := commandutil.Run(ctx, command, s.workDir, &bytes.Buffer{}, &stdout)
	logrus.Debugf("commandutil.Run result: (exit_code: %d, stderr: %s, stdout: %s, err: %s)", result.ExitCode, result.Stderr, result.Stdout, result.Error)

	stdoutDigest := utils.CalSHA256OfInput(result.Stdout)
	stderrDigest := utils.CalSHA256OfInput(result.Stderr)

	if stdoutDigest.GetHash() != EmptySha {
		casCache.Set(ctx, stdoutDigest, result.Stdout)
	}
	if stderrDigest.GetHash() != EmptySha {
		casCache.Set(ctx, stderrDigest, result.Stderr)
	}

	var outputFiles []*repb.OutputFile
	for _, path := range command.GetOutputFiles() {
		fn := filepath.Join(workdir, path)
		fstat, err := os.Stat(fn)
		if err != nil {
			return nil, err
		}
		if fstat.IsDir() {
			return nil, fmt.Errorf("%s is a dir", fn)
		}
		b, err := ioutil.ReadFile(fn)
		if err != nil {
			logrus.WithError(err).Errorf("ioutil.ReadFile")
			return nil, err
		}
		sum := sha256.Sum256(b)
		hash := hex.EncodeToString(sum[:])
		d := &repb.Digest{
			Hash:      hash,
			SizeBytes: fstat.Size(),
		}
		if !action.GetDoNotCache() {
			if err := casCache.Set(ctx, d, b); err != nil {
				return nil, err
			}
		}
		outputFiles = append(outputFiles, &repb.OutputFile{
			Path:           path,
			Digest:         d,
			IsExecutable:   fstat.Mode()&0555 != 0,
			NodeProperties: nil,
		})
	}
	return &repb.ActionResult{
		OutputFiles:             outputFiles,
		OutputFileSymlinks:      nil,
		OutputSymlinks:          nil,
		OutputDirectories:       nil,
		OutputDirectorySymlinks: nil,
		ExitCode:                int32(result.ExitCode),
		StdoutRaw:               result.Stdout,
		StdoutDigest:            stdoutDigest,
		StderrRaw:               result.Stderr,
		StderrDigest:            stderrDigest,
		ExecutionMetadata: &repb.ExecutedActionMetadata{
			Worker:               "main",
			QueuedTimestamp:      timestamppb.Now(),
			WorkerStartTimestamp: timestamppb.Now(),
		},
	}, nil
}
