package grpc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/executor/v1"
	"github.com/ecodeclub/ekit/syncx"
	"github.com/gotomicro/ego/core/elog"
)

type Executor struct {
	executorv1.UnimplementedExecutorServiceServer

	states  *syncx.Map[int64, *executorv1.ExecutionState]
	cancels *syncx.Map[int64, context.CancelFunc]
	logger  *elog.Component
}

func NewExecutor() *Executor {
	return &Executor{
		states:  &syncx.Map[int64, *executorv1.ExecutionState]{},
		cancels: &syncx.Map[int64, context.CancelFunc]{},
		logger:  elog.DefaultLogger,
	}
}

func (s *Executor) Execute(ctx context.Context, request *executorv1.ExecuteRequest) (*executorv1.ExecuteResponse, error) {
	eid := request.GetEid()

	if st, ok := s.states.Load(eid); ok {
		return &executorv1.ExecuteResponse{ExecutionState: st}, nil
	}

	st := executorv1.ExecutionState{
		Id:              eid,
		TaskId:          request.GetTaskId(),
		TaskName:        request.GetTaskName(),
		Status:          executorv1.ExecutionStatus_RUNNING,
		RunningProgress: 0,
	}
	s.states.Store(eid, &st)
	start, err := strconv.Atoi(request.GetParams()["start"])
	if err != nil {
		return nil, err
	}
	end, err := strconv.Atoi(request.GetParams()["end"])
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancels.Store(eid, cancel)
	go s.runTask(runCtx, eid, start, end)

	return &executorv1.ExecuteResponse{ExecutionState: &st}, nil
}

func (s *Executor) runTask(ctx context.Context, eid int64, start, end int) {
	total := end
	progressUnits := start
	incTicker := time.NewTicker(100 * time.Millisecond)
	reportTicker := time.NewTicker(30 * time.Second)
	defer incTicker.Stop()
	defer reportTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			cur, _ := s.states.Load(eid)
			cur.Status = executorv1.ExecutionStatus_FAILED_RESCHEDULABLE
			s.states.Store(eid, cur)
			return
		case <-incTicker.C:
			if progressUnits < total {
				progressUnits++
				pct := int32(progressUnits * 100 / total)
				cur, _ := s.states.Load(eid)
				cur.RunningProgress = pct
				if progressUnits >= total {
					cur.Status = executorv1.ExecutionStatus_SUCCESS
					cur.RunningProgress = 100
					s.states.Store(eid, cur)
					return
				}
				s.logger.Info(fmt.Sprintf("现进度：%d", progressUnits))
				s.states.Store(eid, cur)
			}
		}
	}
}

func (s *Executor) Interrupt(ctx context.Context, request *executorv1.InterruptRequest) (*executorv1.InterruptResponse, error) {
	eid := request.GetEid()
	if cancel, ok := s.cancels.Load(eid); ok {
		cancel()
		cur, _ := s.states.Load(eid)
		return &executorv1.InterruptResponse{
			Success:        true,
			ExecutionState: cur,
		}, nil
	}
	cur, _ := s.states.Load(eid)
	return &executorv1.InterruptResponse{Success: false, ExecutionState: cur}, nil
}

func (s *Executor) Query(ctx context.Context, request *executorv1.QueryRequest) (*executorv1.QueryResponse, error) {
	eid := request.GetEid()
	if st, ok := s.states.Load(eid); ok {
		return &executorv1.QueryResponse{ExecutionState: st}, nil
	}
	st := executorv1.ExecutionState{Id: eid, Status: executorv1.ExecutionStatus_UNKNOWN}
	return &executorv1.QueryResponse{ExecutionState: &st}, nil
}

func (s *Executor) Prepare(ctx context.Context, request *executorv1.PrepareRequest) (*executorv1.PrepareResponse, error) {
	return &executorv1.PrepareResponse{Params: map[string]string{
		"total": "10000",
	}}, nil
}
