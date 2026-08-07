package engine

import (
	"context"
	"os"
	"os/exec"

	"github.com/Duke1616/etask/sdk/executor"
)

// WorkspaceOptions 描述创建单次执行工作区所需的信息。
type WorkspaceOptions struct {
	ExecutionID int64
	Extension   string
	Program     *executor.Program
	Artifacts   executor.ArtifactRoots
}

// Workspace 是语言适配器可使用的单次执行目录。
type Workspace interface {
	// ProgramRoot 返回程序运行根目录。
	ProgramRoot() string
	// EntryPoint 返回程序入口文件绝对路径。
	EntryPoint() string
	// Environment 返回工作区和制品提供的环境变量。
	Environment() []string
	// WriteFile 在工作区写入受控文件并返回绝对路径。
	WriteFile(name string, content []byte, mode os.FileMode) (string, error)
	// Close 清理当前工作区。
	Close() error
}

// WorkspaceFactory 负责创建和回收执行工作区。
type WorkspaceFactory interface {
	// Create 为一次执行创建独立工作区。
	Create(options WorkspaceOptions) (Workspace, error)
	// Prune 清理异常退出遗留的过期工作区。
	Prune() error
	// Validate 校验工作区目录是否可用。
	Validate() error
}

// Input 是传给语言适配器的脚本输入。
type Input struct {
	Args      string
	Variables string
}

// PreparedCommand 是语言适配器构造的待执行命令。
type PreparedCommand struct {
	Command     *exec.Cmd
	Environment []string
}

// Adapter 封装一种脚本语言的元数据和命令构造差异。
type Adapter interface {
	// Name 返回 handler 名称。
	Name() string
	// Description 返回 handler 描述。
	Description() string
	// Extension 返回脚本文件扩展名。
	Extension() string
	// Metadata 返回任务参数元数据。
	Metadata() []executor.Parameter
	// Prepare 根据输入和工作区构造执行命令。
	Prepare(ctx context.Context, workspace Workspace, input Input) (PreparedCommand, error)
	// Validate 校验解释器等语言运行条件。
	Validate() error
}

// ArchiveRecord 描述需要归档的执行现场。
type ArchiveRecord struct {
	ExecutionID int64
	CodeFile    string
	Args        string
	Variables   string
	Failed      bool
}

// Archiver 负责保存和清理脚本执行现场。
type Archiver interface {
	// Archive 按策略保存一次执行现场。
	Archive(record ArchiveRecord) error
	// Prune 清理过期或超限归档。
	Prune() error
	// Validate 校验归档目录是否可用。
	Validate() error
}
