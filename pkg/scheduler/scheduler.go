package scheduler

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/sirupsen/logrus"
	nstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"

	expb "github.com/dashjay/baize/pkg/proto/executor"
	scpb "github.com/dashjay/baize/pkg/proto/scheduler"
	"github.com/dashjay/baize/pkg/utils/status"
)

type Client struct {
	c expb.ExecutorClient
	sync.Mutex
	counter int
}

func (cs *Client) Execute(ctx context.Context, in *expb.Task, opts ...grpc.CallOption) (*expb.TaskResult, error) {
	cs.Lock()
	cs.counter++
	cs.Unlock()
	res, err := cs.c.Execute(ctx, in, opts...)
	cs.Lock()
	cs.counter--
	cs.Unlock()
	return res, err
}

type ClientSets []*Client

func (cs ClientSets) Len() int { return len(cs) }

func (cs ClientSets) Less(i, j int) bool {
	return cs[i].counter < cs[j].counter
}
func (cs ClientSets) Swap(i, j int) {
	cs[i], cs[j] = cs[j], cs[i]
}

type Information struct {
	CPU int
}

type Server struct {
	pq         chan *expb.Task
	clientSets ClientSets
	workerNum  int
	taskMap    map[*expb.Task]chan *expb.TaskResult
	sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		pq:        make(chan *expb.Task, 10),
		workerNum: 10,
		taskMap:   make(map[*expb.Task]chan *expb.TaskResult),
	}
}
func (s *Server) ScheduleTask(action *repb.Action) <-chan *expb.TaskResult {
	task := &expb.Task{Action: action}
	s.pq <- task
	ch := make(chan *expb.TaskResult)
	s.Lock()
	s.taskMap[task] = ch
	s.Unlock()
	return ch
}
func (s *Server) Run(ctx context.Context) {
	for i := 0; i < s.workerNum; i++ {
		go s.RunWorker(ctx)
	}
}
func (s *Server) AcquireClient() (expb.ExecutorClient, error) {
	if len(s.clientSets) == 0 {
		return nil, status.ResourceExhaustedError("no usable executor")
	}
	s.Lock()
	sort.Sort(s.clientSets)
	s.Unlock()
	return s.clientSets[0], nil
}
func (s *Server) RunWorker(ctx context.Context) {
	for {
		t := <-s.pq
		ec, err := s.AcquireClient()
		if err != nil {
			logrus.Warnf("no executor available, sleep for 60 sec and re enquque")
			time.Sleep(60 * time.Second)
			s.pq <- t
			continue
		}

		ch, ok := s.getTaskResultChan(t)
		if !ok {
			logrus.Warnf("get task result chan error, not exists chan")
			continue
		}
		tr, err := ec.Execute(ctx, t)
		if err == nil {
			ch <- tr
		} else {
			// execute error, re enqueue
			if t.GetMaxRetryTime() > 0 {
				t.MaxRetryTime--
				s.pq <- t
			} else {
				tr.Error = "the number of retries exceeded the upper limit"
				ch <- tr
			}
		}
	}
}
func (s *Server) getTaskResultChan(t *expb.Task) (chan *expb.TaskResult, bool) {
	s.RLock()
	ch, ok := s.taskMap[t]
	s.RUnlock()
	return ch, ok
}
func (s *Server) registerExecutor(addr string) error {
	logrus.Infof("register execuor addr %s", addr)
	conn, err := grpc.Dial(addr)
	if err != nil {
		return err
	}
	ec := expb.NewExecutorClient(conn)
	s.Lock()
	s.clientSets = append(s.clientSets, &Client{c: ec})
	s.Unlock()
	return nil
}

func (s *Server) Register(ctx context.Context, req *scpb.RegisterExecutorRequest) (*scpb.RegisterExecutorResponse, error) {
	p, ok := peer.FromContext(ctx)
	if ok {
		return nil, errors.New("get peer from context error")
	}
	ip := net.ParseIP(p.Addr.String()[:strings.Index(p.Addr.String(), ":")])
	err := s.registerExecutor(ip.String() + req.ExecutorAddr)
	if err != nil {
		return nil, err
	}
	return &scpb.RegisterExecutorResponse{Status: &nstatus.Status{}}, nil
}
