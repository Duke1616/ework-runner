package web

type RegisterRunnerReq struct {
	TaskIdentifier string   `json:"task_identifier"`
	TaskSecret     string   `json:"task_secret"`
	Name           string   `json:"name"`
	Tags           []string `json:"tags"`
	Desc           string   `json:"desc"`
}
