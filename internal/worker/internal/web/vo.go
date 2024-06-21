package web

type StartWorkerReq struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Topic string `json:"topic"`
}

type StopWorker struct {
}
