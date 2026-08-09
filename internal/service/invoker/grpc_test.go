package invoker

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCExecutionError(t *testing.T) {
	testCases := []struct {
		name         string
		cause        error
		wantParts    []string
		wantGRPCCode codes.Code
	}{
		{
			name: "连接未就绪",
			cause: status.Error(codes.DeadlineExceeded,
				"context deadline exceeded while waiting for connections to become ready"),
			wantParts:    []string{"A40-executor", "没有可用连接", "READY 节点", "服务注册", "网络连通性"},
			wantGRPCCode: codes.DeadlineExceeded,
		},
		{
			name:         "调用超时",
			cause:        status.Error(codes.DeadlineExceeded, "executor timed out"),
			wantParts:    []string{"A40-executor", "调用执行器服务", "超时"},
			wantGRPCCode: codes.DeadlineExceeded,
		},
		{
			name:         "服务不可用",
			cause:        status.Error(codes.Unavailable, "connection refused"),
			wantParts:    []string{"A40-executor", "当前不可用", "节点健康状态"},
			wantGRPCCode: codes.Unavailable,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := grpcExecutionError("A40-executor", testCase.cause)
			for _, part := range testCase.wantParts {
				if !strings.Contains(err.Error(), part) {
					t.Fatalf("错误信息 %q 不包含 %q", err, part)
				}
			}
			if !errors.Is(err, testCase.cause) {
				t.Fatal("包装后的错误没有保留原始错误链")
			}
			if code := status.Code(err); code != testCase.wantGRPCCode {
				t.Fatalf("gRPC 状态码 = %s, 期望 %s", code, testCase.wantGRPCCode)
			}
		})
	}
}
