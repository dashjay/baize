package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
	"github.com/dashjay/baize/pkg/proto/executor"
	"github.com/dashjay/baize/pkg/utils/digest"
	"github.com/dashjay/baize/pkg/utils/status"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
)

const (
	breakToBrokenTime = 10
	removeAfterBroken = 10
)

// Scheduler implements the ExecutionServer
// 1. Scheduler DO HeartBeat to Executor, for cheking whether it on or off.
// 2. Scheduler implments the ExecutionServer for bazel remote execution. When Exectute called, it will proxy a action to a healthy worker.
type Scheduler struct {
	executors       map[string]*ExecutorEntry
	executorRWLocks sync.RWMutex
	cache           interfaces.Cache
}

func (s *Scheduler) DoHeartBeat() {
	tick := time.NewTicker(time.Second)
	for range tick.C {
		needDelete := make(map[string]bool)

		s.executorRWLocks.RLock()
		for k, v := range s.executors {
			s.executorRWLocks.RUnlock()
			resp, err := v.HeartBeat(context.TODO(), &executor.HeartBeatReq{ExecutorId: k})
			if err == nil {
				v.Property = resp.Propertys
				// remove broken flag, clean brokenTime
				if v.broken {
					v.broken = false
					v.lastBrokenTime = 0
				}
			} else {
				logrus.WithError(err).WithField("executor_id", k).Errorln("do heartbeat error")
				v.Break()
				if v.NeedRemove() {
					needDelete[k] = true
				}
			}
			s.executorRWLocks.RLock()
		}
		s.executorRWLocks.RUnlock()

		if len(needDelete) > 0 {
			s.executorRWLocks.Lock()
			for k := range needDelete {
				delete(s.executors, k)
			}
			s.executorRWLocks.Unlock()
		}
	}
}

func (s *Scheduler) Registr(r *http.Request, rw http.ResponseWriter) {
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		rw.Write([]byte(err.Error()))
		rw.WriteHeader(400)
		return
	}
	r.Body.Close()
	var reg cc.RegisterExecutor
	err = json.Unmarshal(content, &reg)
	if err != nil {
		rw.Write(append([]byte("unmarshal request body error: "), []byte(err.Error())...))
		rw.WriteHeader(500)
		return
	}
	if reg.Id == "" || reg.Addr == "" {
		rw.Write(append([]byte("invalid requests body: "), content...))
		rw.WriteHeader(400)
		return
	}
	s.executorRWLocks.Lock()
	defer s.executorRWLocks.Unlock()
	logger := logrus.WithField("id", reg.Id).WithField("addr", reg.Addr)
	if v, exists := s.executors[reg.Id]; exists {
		if v.cfg.Addr == reg.Addr {
			logger.Debugln("registry again")
			return
		}
		v.cfg.Addr = reg.Addr
		v.Init()
	}
}

func NewScheduler(cache interfaces.Cache) *Scheduler {
	return &Scheduler{executors: make(map[string]*ExecutorEntry), cache: cache}
}

func (s *Scheduler) Execute(req *repb.ExecuteRequest, stream repb.Execution_ExecuteServer) error {
	// action user already upload to ac
	actionDigest := digest.NewResourceName(req.GetActionDigest(), req.GetInstanceName())

	// generate execution id
	executionID, err := actionDigest.UploadString()
	if err != nil {
		return err
	}
	if !req.GetSkipCacheLookup() {
		acCache, err := s.cache.WithIsolation(stream.Context(), interfaces.ActionCacheType, req.GetInstanceName())
		if err != nil {
			return err
		}
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
			stateChangeFn := GetStateChangeFunc(stream, executionID, actionDigest)
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

func (s *Scheduler) WaitExecution(req *repb.WaitExecutionRequest, server repb.Execution_WaitExecutionServer) error {
	return s.waitExecution(req, server, waitOpts{isExecuteRequest: false})
}

func (s *Scheduler) waitExecution(req *repb.WaitExecutionRequest, stream repb.Execution_WaitExecutionServer, opts waitOpts) error {
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
	actionBytes, err := s.cache.Get(ctx, r.GetDigest())
	if err != nil {
		return err
	}
	var action repb.Action
	err = proto.Unmarshal(actionBytes, &action)
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
	resp, err := s.executors["debug"].Execute(ctx, &executor.ExecuteReq{Action: &action})
	if err != nil {
		return err
	}
	data, err := proto.Marshal(resp.Ar)
	if err != nil {
		return status.FailedPreconditionErrorf("marshal action result error: %s", err)
	}
	res, err := digest.ParseUploadResourceName(req.Name)
	if err != nil {
		return status.FailedPreconditionErrorf("parse upload resourceName error: %s", err)
	}
	acCache, err := s.cache.WithIsolation(ctx, interfaces.ActionCacheType, res.GetInstanceName())
	if err != nil {
		return status.FailedPreconditionErrorf("get cache error: %s", err)
	}
	err = acCache.Set(ctx, res.GetDigest(), data)
	if err != nil {
		return status.DataLossErrorf("put action result error: %s", err)
	}
	response, err := marshalAny(&repb.ExecuteResponse{
		Result:  resp.Ar,
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

var _ repb.ExecutionServer = (*Scheduler)(nil)
