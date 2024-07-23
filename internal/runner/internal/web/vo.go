package web

type RegisterRunnerReq struct {
	CodebookUid    string   `json:"codebook_uid"`
	CodebookSecret string   `json:"codebook_secret"`
	Name           string   `json:"name"`
	Tags           []string `json:"tags"`
	Desc           string   `json:"desc"`
}
