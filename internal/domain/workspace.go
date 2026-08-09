package domain

// WorkspaceLayer 表示代码工作区中的来源层。
type WorkspaceLayer string

const (
	// WorkspaceLayerProject 表示当前项目源码层。
	WorkspaceLayerProject WorkspaceLayer = "PROJECT"
	// WorkspaceLayerSystem 表示当前激活的 SYSTEM 制品层。
	WorkspaceLayerSystem WorkspaceLayer = "SYSTEM"
	// WorkspaceLayerDependency 表示当前租户激活的制品依赖层。
	WorkspaceLayerDependency WorkspaceLayer = "DEPENDENCY"
)

// WorkspaceNode 是前端工作区树使用的只读模型。
// RuntimePath 已由后端按照执行目录规则生成，调用方不得再次拼接。
type WorkspaceNode struct {
	Key          string
	SourceID     int64
	ReleaseID    int64
	Digest       string
	ArtifactPath string
	Name         string
	Owner        string
	Kind         CodebookKind
	Scope        CodebookScope
	Layer        WorkspaceLayer
	RuntimePath  string
	Readonly     bool
	ProjectID    int64
	ParentID     int64
	SortNo       int64
	DownloadOnly bool
	Size         int64
	CTime        int64 // 毫秒时间戳；制品虚拟节点使用发布时间。
	UTime        int64 // 毫秒时间戳；不可变制品与 CTime 相同。
	Namespace    string
	Children     []WorkspaceNode
}

// ArtifactManifestFile 表示不可变制品清单中的一个文件。
type ArtifactManifestFile struct {
	Path string
	Hash string
	Size int64
}

// ArtifactContent 表示一个激活制品及其不可变文件清单。
type ArtifactContent struct {
	Release ArtifactRelease
	Files   []ArtifactManifestFile
}
