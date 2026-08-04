package scripts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxConfigResolve(t *testing.T) {
	tests := []struct {
		name      string
		config    SandboxConfig
		euid      int
		isolated  bool
		uid       uint32
		gid       uint32
		wantError string
	}{
		{name: "root 下 auto 默认降权", euid: 0, isolated: true, uid: 65534, gid: 65534},
		{name: "非 root 下 auto 保持兼容", euid: 1000},
		{name: "off 显式关闭", config: SandboxConfig{Mode: SandboxModeOff}, euid: 0},
		{name: "required 接受自定义身份", config: SandboxConfig{Mode: SandboxModeRequired, UID: 1001, GID: 1002}, euid: 0, isolated: true, uid: 1001, gid: 1002},
		{name: "required 拒绝非 root Executor", config: SandboxConfig{Mode: SandboxModeRequired}, euid: 1000, wantError: "要求 Executor 以 root 启动"},
		{name: "拒绝 root 脚本身份", config: SandboxConfig{Mode: SandboxModeRequired, UID: -1, GID: 1000}, euid: 0, wantError: "UID/GID 非法"},
		{name: "拒绝未知模式", config: SandboxConfig{Mode: "strict"}, euid: 0, wantError: "模式非法"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, err := tt.config.resolve(tt.euid)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			if !tt.isolated {
				require.Nil(t, identity)
				return
			}
			require.NotNil(t, identity)
			require.Equal(t, tt.uid, identity.uid)
			require.Equal(t, tt.gid, identity.gid)
		})
	}
}
