package scheduler

import (
	"sync/atomic"
	"time"

	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/proto/executor"
	"google.golang.org/grpc"
)

type ExecutorEntry struct {
	cfg cc.RegisterExecutor
	executor.ExecutorServerClient
	Property *executor.Property

	breakTimes     int64
	lastBrokenTime int64
	broken         bool

	inUse int64
}

func (e *ExecutorEntry) Init() error {
	grpcConn, err := grpc.Dial(e.cfg.Addr)
	if err != nil {
		return err
	}
	e.ExecutorServerClient = executor.NewExecutorServerClient(grpcConn)
	e.breakTimes = 0
	e.broken = false
	e.inUse = 0
	e.lastBrokenTime = 0
	return nil
}

func (e *ExecutorEntry) Break() {
	nv := atomic.AddInt64(&e.breakTimes, 1)
	if nv >= breakToBrokenTime {
		e.lastBrokenTime = time.Now().Unix()
		e.broken = true
	}
}

func (e *ExecutorEntry) NeedRemove() bool {
	return time.Now().Unix()-e.lastBrokenTime > removeAfterBroken
}

func (e *ExecutorEntry) Take() bool {
	if atomic.LoadInt64(&e.inUse)+1 < int64(e.Property.Cpu) {
		atomic.AddInt64(&e.inUse, 1)
		return true
	}
	return false
}

func (e *ExecutorEntry) Return() {
	if atomic.LoadInt64(&e.inUse) < int64(e.Property.Cpu) {
		atomic.AddInt64(&e.inUse, 1)
	}
}
