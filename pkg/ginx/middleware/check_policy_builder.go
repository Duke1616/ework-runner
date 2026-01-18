package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	policyv1 "github.com/Duke1616/ework-runner/api/proto/gen/policy/v1"
	"github.com/ecodeclub/ginx"
	"github.com/ecodeclub/ginx/session"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
)

const Resource = "ALERT"

type CheckPolicyMiddlewareBuilder struct {
	policySvc policyv1.PolicyServiceClient
	logger    *elog.Component
	sp        session.Provider
}

func NewCheckPolicyMiddlewareBuilder(policySvc policyv1.PolicyServiceClient, sp session.Provider) *CheckPolicyMiddlewareBuilder {
	return &CheckPolicyMiddlewareBuilder{
		policySvc: policySvc,
		logger:    elog.DefaultLogger,
		sp:        sp,
	}
}

func (c *CheckPolicyMiddlewareBuilder) Build() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		gCtx := &ginx.Context{Context: ctx}
		sess, err := c.sp.Get(gCtx)
		if err != nil {
			gCtx.AbortWithStatus(http.StatusForbidden)
			c.logger.Debug("用户未登录", elog.FieldErr(err))
			return
		}

		// 获取用户ID
		uid := sess.Claims().Uid
		resp, err := c.policySvc.Authorize(ctx.Request.Context(), &policyv1.AuthorizeReq{
			UserId:   strconv.FormatInt(uid, 10),
			Path:     ctx.Request.URL.Path,
			Method:   ctx.Request.Method,
			Resource: Resource,
		})

		if err != nil {
			fmt.Println("err", err)
		}

		if err != nil || !resp.Allowed {
			gCtx.AbortWithStatus(http.StatusForbidden)
			c.logger.Debug("用户无权限", elog.FieldErr(err))
			return
		}
	}
}
