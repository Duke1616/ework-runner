package codeassist

import (
	"errors"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
	"github.com/ecodeclub/ginx"
)

const (
	SystemErrorCode      = 510001
	InvalidParameterCode = 510002
	ConflictCode         = 510003
)

var systemErrorResult = ginx.Result{Code: SystemErrorCode, Msg: "系统错误"}

func invalidParameterResult(err error) ginx.Result {
	message := strings.TrimPrefix(err.Error(), errs.ErrInvalidParameter.Error()+": ")
	return ginx.Result{Code: InvalidParameterCode, Msg: message}
}

func translateError(err error) ginx.Result {
	switch {
	case errors.Is(err, errs.ErrInvalidParameter):
		return invalidParameterResult(err)
	case errors.Is(err, errs.ErrAIConversationBusy),
		errors.Is(err, errs.ErrAIChangeSetConflict),
		errors.Is(err, errs.ErrCodebookVersionConflict):
		return ginx.Result{Code: ConflictCode, Msg: err.Error()}
	default:
		return systemErrorResult
	}
}

func publicStreamError(err error) string {
	result := translateError(err)
	if result.Code == SystemErrorCode {
		return "AI 请求失败"
	}
	return result.Msg
}
