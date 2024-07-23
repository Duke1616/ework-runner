package web

type WorkerReq struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Topic string `json:"topic"`
}
