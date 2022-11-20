package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/utils/digest"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"
)

func (s *Worker) ensureFiles(ctx context.Context, rootDigest *repb.Digest, base string) error {
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

func (s *Worker) writeFiles(ctx context.Context, files []*repb.FileNode, base string) error {
	writeFile := func(file *repb.FileNode, r io.Reader) error {
		fn := filepath.Join(base, file.GetName())
		if _, err := os.Stat(fn); err != nil && !os.IsNotExist(err) {
			return err
		} else if err == nil {
			return nil
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
		defer fi.Close()
		_, err = io.Copy(fi, r)
		return err
	}
	var smallFiles = make(map[string]*repb.FileNode)
	var largeFiles = make(map[string]*repb.FileNode)
	for i := range files {
		if files[i].Digest.SizeBytes > 4*1024*1024 {
			largeFiles[files[i].Digest.Hash] = files[i]
		} else {
			smallFiles[files[i].Digest.Hash] = files[i]
		}
	}

	if len(smallFiles) > 0 {
		var smallDigestsBundle []*repb.Digest
		for k := range smallFiles {
			smallDigestsBundle = append(smallDigestsBundle, &repb.Digest{Hash: k})
		}
		batchReadResp, err := s.casCli.BatchReadBlobs(ctx, &repb.BatchReadBlobsRequest{Digests: smallDigestsBundle})
		if err != nil {
			return err
		}
		for _, body := range batchReadResp.Responses {
			if fileNode, ok := smallFiles[body.Digest.Hash]; ok {
				err = writeFile(fileNode, bytes.NewReader(body.Data))
				if err != nil {
					return err
				}
			} else {
				logrus.Warnln("file not exists in response")
			}
		}
	}

	if len(largeFiles) > 0 {
		logrus.Warnln("large file")
	}
	return nil
}

func (s *Worker) getDirectoryFromDigest(ctx context.Context, d *repb.Digest) (*repb.Directory, error) {
	content, err := s.ReadCacheTiny(ctx, d)
	if err != nil {
		return nil, err
	}
	var dir repb.Directory
	err = proto.Unmarshal(content, &dir)
	if err != nil {
		return nil, err
	}
	return &dir, nil
}
func (s *Worker) ReadCacheTiny(ctx context.Context, d *repb.Digest) ([]byte, error) {
	resName := digest.NewResourceName(d, "")
	cli, err := s.bytswr.Read(ctx, &bytestream.ReadRequest{ResourceName: resName.DownloadString()})
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
	return resp.Data, nil
}
