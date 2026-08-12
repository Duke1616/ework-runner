package manager

import "github.com/ecodeclub/ginx"

const (
	SystemErrorCode      = 502001
	InvalidParameterCode = 502002
)

func invalidParameterResult(err error) ginx.Result {
	return ginx.Result{Code: InvalidParameterCode, Msg: err.Error()}
}

var (
	SystemError = ErrorCode{Code: SystemErrorCode, Msg: "系统错误"}

	systemErrorResult = ginx.Result{
		Code: SystemError.Code,
		Msg:  SystemError.Msg,
	}
)

type ErrorCode struct {
	Code int
	Msg  string
}
