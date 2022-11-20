package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/proto/executor"
	"github.com/dashjay/baize/pkg/utils"
	"github.com/dashjay/baize/pkg/utils/commandutil"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"
)

type Worker struct {
	workDir string
	bytswr  bytestream.ByteStreamClient
	casCli  repb.ContentAddressableStorageClient
}

var _ executor.ExecutorServerServer = (*Worker)(nil)

func (w *Worker) HeartBeat(ctx context.Context, req *executor.HeartBeatReq) (*executor.HeartBeatResp, error) {
	return nil, nil
}

func (w *Worker) Execute(ctx context.Context, req *executor.ExecuteReq) (*executor.ExecuteResp, error) {
	if err := w.ensureFiles(ctx, req.Action.GetInputRootDigest(), w.workDir); err != nil {
		logrus.WithError(err).Errorf("ensureFiles")
		return nil, err
	}
	resName, err := cc.GetReadResourceName("", req.Action.CommandDigest.Hash, req.Action.CommandDigest.SizeBytes, "")
	if err != nil {
		return nil, err

	}
	cli, err := w.bytswr.Read(ctx, &bytestream.ReadRequest{ResourceName: resName})
	if err != nil {
		return nil, err
	}
	resp, err := cli.Recv()
	if err != nil {
		return nil, err
	}
	err = cli.CloseSend()
	if err != nil {
		return nil, err
	}

	var command repb.Command
	err = proto.Unmarshal(resp.Data, &command)
	if err != nil {
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
		base := filepath.Join(w.workDir, filepath.Dir(file))
		if err := os.MkdirAll(base, os.ModePerm); err != nil {
			logrus.WithError(err).Errorf("os.MkdirAll")
			return nil, err
		}
	}

	var stdout bytes.Buffer
	result := commandutil.Run(ctx, &command, w.workDir, &bytes.Buffer{}, &stdout)
	logrus.Debugf("commandutil.Run result: (exit_code: %d, stderr: %s, stdout: %s, err: %s)", result.ExitCode, result.Stderr, result.Stdout, result.Error)

	stdoutDigest := utils.CalSHA256OfInput(result.Stdout)
	stderrDigest := utils.CalSHA256OfInput(result.Stderr)

	if stdoutDigest.GetHash() != cc.EmptySha {
		// casCache.Set(ctx, stdoutDigest, result.Stdout)
	}
	if stderrDigest.GetHash() != cc.EmptySha {
		// casCache.Set(ctx, stderrDigest, result.Stderr)
	}

	var outputFiles []*repb.OutputFile
	for _, path := range command.GetOutputFiles() {
		fn := filepath.Join(w.workDir, path)
		fstat, err := os.Stat(fn)
		if err != nil {
			return nil, err
		}
		if fstat.IsDir() {
			return nil, fmt.Errorf("%s is a dir", fn)
		}
		fd, err := os.Open(fn)
		if err != nil {
			return nil, err
		}
		sum := sha256.New()
		_, err = io.Copy(sum, fd)
		if err != nil {
			return nil, err
		}
		_, err = fd.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}
		hash := hex.EncodeToString(sum.Sum(nil))
		d := &repb.Digest{
			Hash:      hash,
			SizeBytes: fstat.Size(),
		}
		if !req.Action.GetDoNotCache() {
			wr, err := w.bytswr.Write(ctx)
			if err != nil {
				logrus.WithError(err).Errorln("create bytestream Writer error")
			} else {
				resName, err := cc.GetDefaultWriteResourceName(uuid.NewString(), d.Hash, d.SizeBytes)
				if err != nil {
					return nil, err
				}
				if fstat.Size() < cc.DefaultReadCapacity {
					var buf = make([]byte, fstat.Size())
					_, err = fd.Write(buf)
					if err != nil {
						return nil, err
					}
					err = wr.Send(&bytestream.WriteRequest{ResourceName: resName, WriteOffset: 0, FinishWrite: true, Data: buf})
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
					for commited < fstat.Size() {
						n, err := fd.Write(buf)
						if err != nil {
							return nil, err
						}
						finishWrite := commited+int64(n) >= fstat.Size()
						err = wr.Send(&bytestream.WriteRequest{ResourceName: resName, WriteOffset: int64(commited), FinishWrite: finishWrite, Data: buf[:n]})
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
			}
		}
		outputFiles = append(outputFiles, &repb.OutputFile{
			Path:           path,
			Digest:         d,
			IsExecutable:   fstat.Mode()&0555 != 0,
			NodeProperties: nil,
		})
	}
	return &executor.ExecuteResp{Ar: &repb.ActionResult{
		OutputFiles:  outputFiles,
		ExitCode:     int32(result.ExitCode),
		StdoutDigest: stdoutDigest,
		StderrDigest: stderrDigest,
	}}, nil
}
