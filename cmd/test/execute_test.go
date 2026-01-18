package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/repository/dao"
	"github.com/Duke1616/ework-runner/ioc"
	"github.com/Duke1616/ework-runner/pkg/sqlx"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// 这个方法用于初始化任务,需要提前将执行节点，调度节点启动
func TestDemoStart(t *testing.T) {
	initViper()

	// 初始化db
	db := ioc.InitDB()
	taskDAO := dao.NewGORMTaskDAO(db)
	// 初始化task
	now := time.Now()
	taskName := fmt.Sprintf("task_%s", uuid.New().String())
	_, err := taskDAO.Create(t.Context(), dao.Task{
		Name:     taskName,
		CronExpr: "0 0 * * * ?",
		Type:     domain.TaskTypeOneTime.String(),
		GrpcConfig: sqlx.JSONColumn[domain.GrpcConfig]{
			Valid: true,
			Val: domain.GrpcConfig{
				ServiceName: "execute",
				HandlerName: "demo",
			},
		},
		ScheduleParams: sqlx.JSONColumn[map[string]string]{
			Valid: true,
			Val: map[string]string{
				"start": "0",
				"end":   "33",
			},
		},
		Status:   domain.TaskStatusActive.String(),
		Version:  1,
		NextTime: now.Add(3 * time.Second).UnixMilli(),
	})
	require.NoError(t, err)
}

func TestShellStart(t *testing.T) {
	initViper()

	// 初始化db
	db := ioc.InitDB()
	taskDAO := dao.NewGORMTaskDAO(db)
	// 初始化task
	now := time.Now()
	taskName := fmt.Sprintf("task_shell_%s", uuid.New().String())

	// NOTE: Shell 脚本示例 - 模拟真实的运维场景(Kubernetes Pod 操作)
	shellScript := `#!/bin/bash
# 脚本描述: 演示 ework-runner 的 Shell 任务执行能力

## 传递工单提交信息
args=$1

## 为了防止重复编写脚本,设定环境变量机制,变量请通过 Runner 模块进行自定义配置
## 存储在临时文件中,通过 source 导入
## 使用注入变量: KUBECONFIG_PATH, OPERATOR_NAME, ENVIRONMENT
vars=$2
source $vars

echo "========================================="
echo "开始执行 Shell 任务"
echo "========================================="

## 全局变量
date=$(date +%Y%m%d%H%M%S)
log_file="task-${date}.log"

## 从 JSON 参数中提取业务参数
pod_name=$(echo "$args" | jq -r '.pod_name')
namespace=$(echo "$args" | jq -r '.namespace')
user_info=$(echo "$args" | jq -r '.user_info')

echo "任务参数信息:"
echo "  - 命名空间: $namespace"
echo "  - Pod 名称: $pod_name"
echo "  - 用户信息: $user_info"
echo ""

echo "环境变量信息:"
echo "  - 操作人员: $OPERATOR_NAME"
echo "  - 运行环境: $ENVIRONMENT"
echo "  - Kubeconfig: $KUBECONFIG_PATH"
echo ""

## 模拟 Kubernetes 操作
echo "开始执行 Kubernetes 操作..."
echo "[1/5] 检查 Pod 状态"
# kubectl --kubeconfig=$KUBECONFIG_PATH get pod $pod_name -n $namespace
echo "  ✓ Pod 状态检查完成"
sleep 1

echo "[2/5] 获取 Pod 详细信息"
# kubectl --kubeconfig=$KUBECONFIG_PATH describe pod $pod_name -n $namespace
echo "  ✓ Pod 详细信息获取完成"
sleep 1

echo "[3/5] 查看 Pod 日志"
# kubectl --kubeconfig=$KUBECONFIG_PATH logs $pod_name -n $namespace --tail=100
echo "  ✓ Pod 日志查看完成"
sleep 1

echo "[4/5] 检查资源使用情况"
# kubectl --kubeconfig=$KUBECONFIG_PATH top pod $pod_name -n $namespace
echo "  ✓ 资源使用情况检查完成"
sleep 1

echo "[5/5] 生成操作报告"
cat > $log_file <<EOF
操作报告
========================================
操作时间: $(date '+%Y-%m-%d %H:%M:%S')
操作人员: $OPERATOR_NAME
运行环境: $ENVIRONMENT
命名空间: $namespace
Pod 名称: $pod_name
操作状态: 成功
========================================
EOF
echo "  ✓ 操作报告生成完成: $log_file"

echo ""
echo "========================================="
echo "Shell 任务执行完成!"
echo "========================================="
exit 0
`

	// NOTE: 参数配置 - 以 JSON 格式传递,模拟真实业务场景
	args := `{
		"namespace": "sre",
		"pod_name": "nginx",
		"user_info": "{\"id\":1,\"username\":\"demo\",\"display_name\":\"演示用户\",\"email\":\"demo@example.com\"}"
	}`

	// NOTE: 变量配置 - 以 JSON 格式传递,执行时会转换为 KEY=VALUE 格式
	// 这些变量会被写入临时文件,然后通过 source 命令导入到 shell 脚本中
	variables := `[
		{"key": "KUBECONFIG_PATH", "value": "/home/demo/.kube/config"},
		{"key": "OPERATOR_NAME", "value": "demo"},
		{"key": "ENVIRONMENT", "value": "development"}
	]`

	_, err := taskDAO.Create(t.Context(), dao.Task{
		Name:     taskName,
		CronExpr: "0 0 * * * ?",
		Type:     domain.TaskTypeOneTime.String(),
		GrpcConfig: sqlx.JSONColumn[domain.GrpcConfig]{
			Valid: true,
			Val: domain.GrpcConfig{
				ServiceName: "execute",
				HandlerName: "shell",
				Params: map[string]string{
					"code":      shellScript,
					"args":      args,
					"variables": variables,
				},
			},
		},
		Status:   domain.TaskStatusActive.String(),
		Version:  1,
		NextTime: now.Add(3 * time.Second).UnixMilli(),
	})
	require.NoError(t, err)
	t.Logf("创建 Shell 任务成功: %s", taskName)
}

func initViper() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	f, err := os.Open(dir + "/../../config/config.yaml")
	if err != nil {
		panic(err)
	}
	viper.SetConfigFile(f.Name())
	viper.WatchConfig()
	err = viper.ReadInConfig()
}
