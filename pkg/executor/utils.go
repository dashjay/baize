package executor

import (
	"context"
	"io"
	"os"
	"path/filepath"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
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
		if d.GetHash() != cc.EmptySha {
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

func (s *Worker) getDirectoryFromDigest(ctx context.Context, d *repb.Digest) (*repb.Directory, error) {
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
func (s *Worker) ReadCacheTiny(ctx context.Context, d *repb.Digest) ([]byte, error) {
	resName,err := digest.NewResourceName(d,)
	if err!=nil{
		return nil,err
	}
	cli, err := s.bytswr.Read(ctx,&bytestream.ReadRequest{ResourceName: })
	if err != nil {
		return nil, err
	}
}
