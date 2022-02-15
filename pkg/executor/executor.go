package executor

import (
	"context"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/dashjay/baize/pkg/config"
	expb "github.com/dashjay/baize/pkg/proto/executor"
	scpb "github.com/dashjay/baize/pkg/proto/scheduler"
)

type Server struct {
	listenAddr string
	grpcServer *grpc.Server
	sc         scpb.SchedulerClient
}

func (s *Server) Execute(ctx context.Context, task *expb.Task) (*expb.TaskResult, error) {
	panic("implement me")
}

func New(cfg *config.Configure) (*Server, error) {
	srv := grpc.NewServer()
	es := &Server{listenAddr: cfg.GetExecutorConfig().ListenAddr, grpcServer: srv}
	expb.RegisterExecutorServer(es.grpcServer, es)
	return es, nil
}

func (s *Server) Register(localAddr string) error {
	ctx := context.Background()
	resp, err := s.sc.Register(ctx, &scpb.RegisterExecutorRequest{
		ExecutorInfo: nil,
		ExecutorAddr: localAddr,
	})
	if err != nil {
		return err
	}
	logrus.Infof("resp.Status.String(): %s", resp.Status.String())
	return nil
}

func (s *Server) Run() error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	defer lis.Close()
	logrus.Infof("baize server remote execuotor listen at addr %s", s.listenAddr)

	for i := 0; i < 10; i++ {
		if err := s.Register(s.listenAddr); err != nil {
			logrus.Warnf("register this client to server error: %s", err)
			time.Sleep(5 * time.Second)
			continue
		}
	}
	return s.grpcServer.Serve(lis)
}
