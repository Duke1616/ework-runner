package task

import "github.com/ecodeclub/ginx"

const (
	SystemErrorCode = 502001
)

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
