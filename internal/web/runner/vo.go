package runner

import "encoding/json"

type RegisterRunnerReq struct {
	Name              string                     `json:"name"`
	CodebookID        int64                      `json:"codebook_id"`
	ProgramKind       string                     `json:"program_kind"`
	CodebookSecret    string                     `json:"codebook_secret"`
	Kind              string                     `json:"kind"`
	Target            string                     `json:"target"`
	Handler           string                     `json:"handler"`
	Tags              []string                   `json:"tags"`
	Desc              string                     `json:"desc"`
	ParameterDefaults map[string]json.RawMessage `json:"parameter_defaults"`
	Variables         []Variable                 `json:"variables"`
}

type UpdateRunnerReq struct {
	ID                int64                      `json:"id"`
	Name              string                     `json:"name"`
	CodebookID        int64                      `json:"codebook_id"`
	ProgramKind       string                     `json:"program_kind"`
	CodebookSecret    string                     `json:"codebook_secret"`
	Kind              string                     `json:"kind"`
	Target            string                     `json:"target"`
	Handler           string                     `json:"handler"`
	Tags              []string                   `json:"tags"`
	Desc              string                     `json:"desc"`
	ParameterDefaults map[string]json.RawMessage `json:"parameter_defaults"`
	Variables         []Variable                 `json:"variables"`
}

type ListRunnerByIDsReq struct {
	IDs []int64 `json:"ids"`
}

type Variable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

type Page struct {
	Offset int64 `json:"offset,omitempty"`
	Limit  int64 `json:"limit,omitempty"`
}

type ListRunnerReq struct {
	Page
	Keyword string `json:"keyword"`
	Kind    string `json:"kind"`
}

type ListExcludeCodebookIDReq struct {
	Page
	CodebookID int64  `json:"codebook_id"`
	Keyword    string `json:"keyword"`
	Kind       string `json:"kind"`
}

type RunnerVO struct {
	ID                int64                      `json:"id"`
	Name              string                     `json:"name"`
	Kind              string                     `json:"kind"`
	CodebookID        int64                      `json:"codebook_id"`
	ProgramKind       string                     `json:"program_kind"`
	Target            string                     `json:"target"`
	Handler           string                     `json:"handler"`
	Tags              []string                   `json:"tags"`
	Variables         []Variable                 `json:"variables"`
	ParameterDefaults map[string]json.RawMessage `json:"parameter_defaults"`
	Desc              string                     `json:"desc"`
	CTime             int64                      `json:"ctime"`
	UTime             int64                      `json:"utime"`
}

type ListRunnersResp struct {
	Total   int64      `json:"total"`
	Runners []RunnerVO `json:"runners"`
}
