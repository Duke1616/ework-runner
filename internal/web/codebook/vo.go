package codebook

type CreateReq struct {
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Code      string `json:"code"`
	ParentID  int64  `json:"parent_id"`
	Scope     string `json:"scope"`
	Kind      string `json:"kind"`
	SortNo    int64  `json:"sort_no"`
}

type UpdateReq struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Code      string `json:"code"`
	Scope     string `json:"scope"`
	SortNo    int64  `json:"sort_no"`
}

// RenameReq 重命名代码资源节点，文件和目录均支持。
type RenameReq struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type SortReq struct {
	ID             int64 `json:"id"`
	TargetParentID int64 `json:"target_parent_id"`
	TargetPosition int64 `json:"target_position"`
}

type CreateVersionReq struct {
	NodeID  int64  `json:"node_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type UseVersionReq struct {
	NodeID    int64 `json:"node_id"`
	VersionID int64 `json:"version_id"`
}

type ListVersionsReq struct {
	NodeID int64 `json:"node_id"`
}

type Page struct {
	Offset int64 `json:"offset,omitempty"`
	Limit  int64 `json:"limit,omitempty"`
}

type ListReq struct {
	Page
}

type ListProjectsReq struct {
	Page
	Keyword string `json:"keyword,omitempty"`
	Status  string `json:"status,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

type ListReferenceProjectsReq struct {
	Page
	Keyword          string `json:"keyword,omitempty"`
	ExcludeProjectID int64  `json:"exclude_project_id,omitempty"`
}

type ProjectDeleteReq struct {
	ProjectName string `json:"project_name"`
}

type ProjectDeleteImpact struct {
	TaskCount                    int64 `json:"task_count"`
	ActiveTaskCount              int64 `json:"active_task_count"`
	CodebookNodeCount            int64 `json:"codebook_node_count"`
	CodebookVersionCount         int64 `json:"codebook_version_count"`
	ArtifactReleaseCount         int64 `json:"artifact_release_count"`
	ArtifactReleaseBytes         int64 `json:"artifact_release_bytes"`
	RetainedArtifactReleaseCount int64 `json:"retained_artifact_release_count"`
	ProjectSourceCount           int64 `json:"project_source_count"`
	ProjectSourceBytes           int64 `json:"project_source_bytes"`
	RetainedProjectSourceCount   int64 `json:"retained_project_source_count"`
	AIConversationCount          int64 `json:"ai_conversation_count"`
}

type ChildrenReq struct {
	ProjectID int64  `json:"project_id"`
	ParentID  int64  `json:"parent_id"`
	Scope     string `json:"scope,omitempty"`
}

type WorkspaceFileReq struct {
	ProjectID    int64  `json:"project_id"`
	ReleaseID    int64  `json:"release_id"`
	Digest       string `json:"digest"`
	ArtifactPath string `json:"artifact_path"`
}

type WorkspaceFileResp struct {
	Code string `json:"code"`
}

type WorkspaceTreeResp struct {
	Nodes []WorkspaceNode `json:"nodes"`
}

type ImportResp struct {
	FileCount      int `json:"file_count"`
	DirectoryCount int `json:"directory_count"`
}

type WorkspaceNode struct {
	Key          string          `json:"key"`
	SourceID     int64           `json:"source_id"`
	ReleaseID    int64           `json:"release_id"`
	Digest       string          `json:"digest"`
	ArtifactPath string          `json:"artifact_path"`
	Name         string          `json:"name"`
	Owner        string          `json:"owner"`
	Kind         string          `json:"kind"`
	Scope        string          `json:"scope"`
	Layer        string          `json:"layer"`
	RuntimePath  string          `json:"runtime_path"`
	Readonly     bool            `json:"readonly"`
	ProjectID    int64           `json:"project_id"`
	ParentID     int64           `json:"parent_id"`
	SortNo       int64           `json:"sort_no"`
	DownloadOnly bool            `json:"download_only"`
	Size         int64           `json:"size"`
	CTime        int64           `json:"ctime"`
	UTime        int64           `json:"utime"`
	Namespace    string          `json:"namespace"`
	Children     []WorkspaceNode `json:"children"`
}

type Codebook struct {
	ID               int64  `json:"id"`
	TenantID         int64  `json:"tenant_id"`
	Scope            string `json:"scope"`
	ProjectID        int64  `json:"project_id"`
	ParentID         int64  `json:"parent_id"`
	PathIDs          string `json:"path_ids"`
	Depth            int    `json:"depth"`
	Name             string `json:"name"`
	Owner            string `json:"owner"`
	Kind             string `json:"kind"`
	SortNo           int64  `json:"sort_no"`
	Code             string `json:"code"`
	Size             int64  `json:"size"`
	DownloadOnly     bool   `json:"download_only"`
	Secret           string `json:"secret"`
	CurrentVersionID int64  `json:"current_version_id"`
	CurrentVersionNo int64  `json:"current_version_no"`
	CTime            int64  `json:"ctime"`
	UTime            int64  `json:"utime"`
}

type ListCodebooksResp struct {
	Total     int64      `json:"total"`
	Codebooks []Codebook `json:"codebooks"`
}

type Version struct {
	ID           int64  `json:"id"`
	NodeID       int64  `json:"node_id"`
	TenantID     int64  `json:"tenant_id"`
	Scope        string `json:"scope"`
	VersionNo    int64  `json:"version_no"`
	Code         string `json:"code"`
	Hash         string `json:"hash"`
	Message      string `json:"message"`
	AuthorUserID int64  `json:"author_user_id"`
	CTime        int64  `json:"ctime"`
}

type ListVersionsResp struct {
	Versions []Version `json:"versions"`
}

type CreateProjectReq struct {
	Name              string `json:"name"`
	Desc              string `json:"desc"`
	ArtifactEnabled   bool   `json:"artifact_enabled"`
	ArtifactNamespace string `json:"artifact_namespace"`
}

type UpdateProjectReq struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Desc              string `json:"desc"`
	SortNo            int64  `json:"sort_no"`
	ArtifactEnabled   bool   `json:"artifact_enabled"`
	ArtifactNamespace string `json:"artifact_namespace"`
}

type Project struct {
	ID                int64  `json:"id"`
	TenantID          int64  `json:"tenant_id"`
	Scope             string `json:"scope"`
	Name              string `json:"name"`
	Desc              string `json:"desc"`
	SortNo            int64  `json:"sort_no"`
	Status            string `json:"status"`
	ArtifactEnabled   bool   `json:"artifact_enabled"`
	ArtifactNamespace string `json:"artifact_namespace"`
	SourceRevision    int64  `json:"source_revision"`
	CTime             int64  `json:"ctime"`
	UTime             int64  `json:"utime"`
}

type ListProjectsResp struct {
	Total    int64     `json:"total"`
	Projects []Project `json:"projects"`
}
