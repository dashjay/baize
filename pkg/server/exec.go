package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ExecutorServer) Execute(in *repb.ExecuteRequest, server repb.Execution_ExecuteServer) error {
	if strings.ContainsRune(in.InstanceName, '|') {
		logrus.Errorf("Contains |")
		return status.Errorf(codes.InvalidArgument, "Instance name cannot contain a pipe character")
	}

	// Response
	eom := &repb.ExecuteOperationMetadata{
		Stage:            repb.ExecutionStage_QUEUED,
		ActionDigest:     in.GetActionDigest(),
		StdoutStreamName: "",
		StderrStreamName: "",
	}
	eomAsPBAny, err := marshalAny(eom)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	// Include the response message in the longrunning operation message
	op := &longrunning.Operation{
		Name:     in.GetActionDigest().GetHash(),
		Metadata: eomAsPBAny,
		Done:     false,
	}
	if err := server.Send(op); err != nil {
		logrus.WithError(err).Errorf("server.Send repb.ExecutionStage_QUEUED")
		return err
	}
	return nil
}

func (s *ExecutorServer) WaitExecution(in *repb.WaitExecutionRequest, server repb.Execution_WaitExecutionServer) error {
	ctx := context.Background()
	digest := &repb.Digest{
		Hash: in.GetName(),
	}
	data, err := s.cache.Get(ctx, digest)
	if err != nil {
		msg := fmt.Sprintf("Failed to get data %s from storage", in.GetName())
		return handleGrpcError(codes.Internal, err, msg)
	}
	digest.SizeBytes = int64(len(data))
	action := &repb.Action{}
	if err := proto.Unmarshal(data, action); err != nil {
		return handleGrpcError(codes.Internal, err, "Failed to unmarshal proto data")
	}

	// Response
	eom := &repb.ExecuteOperationMetadata{
		Stage:            repb.ExecutionStage_EXECUTING,
		ActionDigest:     digest,
		StdoutStreamName: "",
		StderrStreamName: "",
	}
	eomAsPBAny, err := marshalAny(eom)
	if err != nil {
		return handleGrpcError(codes.Internal, err, "can not marshal eom")
	}
	// Include the response message in the longrunning operation message
	op := &longrunning.Operation{
		Name:     in.GetName(),
		Metadata: eomAsPBAny,
		Done:     false,
	}
	if err := server.Send(op); err != nil {
		return err
	}
	actionResult, err := s.runWorker(ctx, action, s.workDir)
	if err != nil {
		logrus.WithError(err).Errorf("runWorker")
		return err
	}
	if err := s.PutActionResultByDigest(ctx, digest, actionResult); err != nil {
		logrus.WithError(err).Errorf("PutActionResultByDigest")
		return err
	}
	response, err := marshalAny(&repb.ExecuteResponse{
		Result:       actionResult,
		CachedResult: false,
		Status:       nil,
		ServerLogs:   nil,
		Message:      "action executing",
	})
	if err != nil {
		logrus.WithError(err).Errorf("marshalAny executeResponse")
		return err
	}

	eom.Stage = repb.ExecutionStage_COMPLETED
	op.Result = &longrunning.Operation_Response{Response: response}
	op.Done = true

	return server.Send(op)
}

func (s *ExecutorServer) ensureFiles(ctx context.Context, rootDigest *repb.Digest, base string) error {
	rootDir, err := s.GetDirectoryFromDigest(ctx, rootDigest)
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
	for _, file := range files {
		d := file.GetDigest()
		var data []byte
		var err error
		// Get file contents
		if d.GetHash() != EmptySha {
			data, err = s.cache.Get(ctx, d)
			if err != nil {
				logrus.WithError(err).Error("writeFiles")
				return err
			}
		}

		fn := filepath.Join(base, file.GetName())

		if _, err := os.Stat(fn); err != nil && !os.IsNotExist(err) {
			return err
		} else if err == nil {
			continue
		}

		if err := os.MkdirAll(base, os.ModePerm); err != nil {
			return err
		}
		mode := os.FileMode(0644)
		if file.GetIsExecutable() {
			mode = os.FileMode(0555)
		}
		if err := ioutil.WriteFile(fn, data, mode); err != nil {
			return err
		}
	}
	return nil
}

func (s *ExecutorServer) runWorker(ctx context.Context, action *repb.Action, workdir string) (*repb.ActionResult, error) {
	if err := s.ensureFiles(ctx, action.GetInputRootDigest(), workdir); err != nil {
		logrus.WithError(err).Errorf("ensureFiles")
		return nil, err
	}

	// repb.Command
	command, err := s.GetCommandFromDigest(ctx, action.GetCommandDigest())
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

	// TODO: GetOutputDirectories and GetOutputNodeProperties

	// make cmd
	args := command.GetArguments()
	cmd := exec.Command(args[0], args[1:]...)
	for _, env := range command.GetEnvironmentVariables() {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", env.Name, env.Value))
	}
	cmd.Dir = command.GetWorkingDirectory()
	if cmd.Dir == "" {
		cmd.Dir = workdir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logrus.WithError(err).Errorf("cmd run: %+v", cmd.Env)
		return nil, err
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
		digest := &repb.Digest{
			Hash:      hash,
			SizeBytes: fstat.Size(),
		}
		if !action.GetDoNotCache() {
			if err := s.cache.Set(ctx, digest, b); err != nil {
				return nil, err
			}
		}

		outputFiles = append(outputFiles, &repb.OutputFile{
			Path:           path,
			Digest:         digest,
			IsExecutable:   true,
			NodeProperties: nil,
		})
	}
	return &repb.ActionResult{
		OutputFiles:             outputFiles,
		OutputFileSymlinks:      nil,
		OutputSymlinks:          nil,
		OutputDirectories:       nil,
		OutputDirectorySymlinks: nil,
		ExitCode:                0,
		StdoutRaw:               nil,
		StdoutDigest:            nil,
		StderrRaw:               nil,
		StderrDigest:            nil,
		ExecutionMetadata:       nil,
	}, nil
}
