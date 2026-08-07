package preview

import "github.com/Duke1616/etask/internal/domain"

type Variable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

type RunReq struct {
	RunnerID int64        `json:"runner_id"`
	Program  *ProgramSpec `json:"program"`
	// CodebookID 和 Code 仅用于兼容旧版前端请求，核心服务不再使用这两个字段。
	CodebookID          int64      `json:"codebook_id,omitempty"`
	Code                string     `json:"code,omitempty"`
	Args                string     `json:"args"`
	Variables           []Variable `json:"variables"`
	MaxExecutionSeconds int64      `json:"max_execution_seconds"`
}

func (r RunReq) resolvedProgram() *ProgramSpec {
	if r.Program != nil {
		return r.Program
	}
	if r.CodebookID <= 0 || r.Code == "" {
		return nil
	}
	return &ProgramSpec{Kind: string(domain.ProgramInline), Inline: &InlineProgramSpec{Code: r.Code}}
}

type ProgramSpec struct {
	Kind    string              `json:"kind"`
	Inline  *InlineProgramSpec  `json:"inline,omitempty"`
	Project *ProjectProgramSpec `json:"project,omitempty"`
}

type InlineProgramSpec struct {
	Code       string `json:"code,omitempty"`
	CodebookID int64  `json:"codebook_id,omitempty"`
}

type ProjectProgramSpec struct {
	EntryCodebookID int64 `json:"entry_codebook_id"`
}

type StatusReq struct {
	ExecutionID int64 `json:"execution_id"`
}

type LogsReq struct {
	ExecutionID int64 `json:"execution_id"`
	MinID       int64 `json:"min_id"`
	Limit       int   `json:"limit"`
}

type ExecutionVO struct {
	ID              int64  `json:"id"`
	TaskName        string `json:"task_name"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	Status          string `json:"status"`
	RunningProgress int32  `json:"running_progress"`
	ExecutorNodeID  string `json:"executor_node_id"`
	TaskResult      string `json:"task_result"`
	CTime           int64  `json:"ctime"`
}

type LogVO struct {
	ID          int64  `json:"id"`
	ExecutionID int64  `json:"execution_id"`
	Content     string `json:"content"`
	CTime       int64  `json:"ctime"`
}

type LogsResp struct {
	Total int64   `json:"total"`
	Logs  []LogVO `json:"logs"`
}
