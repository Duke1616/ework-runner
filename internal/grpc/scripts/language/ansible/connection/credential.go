package connection

// AuthenticationKind 标识 SSH 认证材料的类型。
type AuthenticationKind string

const (
	// AuthenticationKindPrivateKey 表示无口令 SSH 私钥认证。
	AuthenticationKindPrivateKey AuthenticationKind = "PRIVATE_KEY"
	// AuthenticationKindPassword 表示 SSH 密码认证。
	AuthenticationKindPassword AuthenticationKind = "PASSWORD"
)

// Authentication 是可扩展的 SSH 认证材料。
type Authentication interface {
	// Kind 返回认证材料类型，用于选择对应的安全物化方式。
	Kind() AuthenticationKind
	// Clear 尽可能覆盖当前进程内持有的敏感字节。
	Clear()
}

// PrivateKeyAuthentication 保存一次解析出的无口令 SSH 私钥。
type PrivateKeyAuthentication struct {
	PrivateKey []byte
}

// Kind 返回 SSH 私钥认证类型。
func (*PrivateKeyAuthentication) Kind() AuthenticationKind {
	return AuthenticationKindPrivateKey
}

// Clear 覆盖内存中的私钥字节。
func (a *PrivateKeyAuthentication) Clear() {
	if a == nil {
		return
	}
	clearBytes(a.PrivateKey)
}

// PasswordAuthentication 保存一次解析出的 SSH 密码。
type PasswordAuthentication struct {
	Password []byte
}

// Kind 返回 SSH 密码认证类型。
func (*PasswordAuthentication) Kind() AuthenticationKind {
	return AuthenticationKindPassword
}

// Clear 覆盖内存中的密码字节。
func (a *PasswordAuthentication) Clear() {
	if a == nil {
		return
	}
	clearBytes(a.Password)
}

// Credential 描述一次 SSH 连接需要的用户名和认证材料。
type Credential struct {
	Username       string
	Authentication Authentication
}

// Clear 清理凭据持有的敏感认证材料。
func (c *Credential) Clear() {
	if c.Authentication != nil {
		c.Authentication.Clear()
	}
}

// CredentialProvider 负责根据非敏感引用提供 SSH 凭据。
type CredentialProvider interface {
	// References 返回可供任务选择的非敏感凭据引用，并保证顺序稳定。
	References() []string
	// Resolve 根据引用读取一次执行所需的凭据，调用方负责及时清理敏感材料。
	Resolve(reference string) (Credential, error)
	// Validate 校验 Provider 配置和当前可用的全部凭据。
	Validate() error
}

// HostKeyProvider 负责提供 SSH 服务端身份信任数据。
type HostKeyProvider interface {
	// KnownHosts 返回受信任的 OpenSSH known_hosts 文件内容。
	KnownHosts() ([]byte, error)
	// Validate 校验主机信任配置和文件安全属性。
	Validate() error
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
