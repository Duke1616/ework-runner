package connection

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maximumProtectedFileSize = 1 << 20

type protectedFilePolicy struct {
	description          string
	forbiddenPermissions os.FileMode
}

var (
	privateKeyFilePolicy = protectedFilePolicy{
		description: "私钥", forbiddenPermissions: 0o077,
	}
	passwordFilePolicy = protectedFilePolicy{
		description: "密码", forbiddenPermissions: 0o077,
	}
	knownHostsFilePolicy = protectedFilePolicy{
		description: "known_hosts", forbiddenPermissions: 0o022,
	}
)

func readFileWithinRoot(root, name string, policy protectedFilePolicy) ([]byte, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("解析凭据根目录失败: %w", err)
	}
	resolvedName, err := filepath.EvalSymlinks(filepath.Join(root, name))
	if err != nil {
		return nil, fmt.Errorf("解析文件失败: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedName)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, fmt.Errorf("文件超出凭据根目录")
	}
	return readProtectedFile(resolvedName, policy)
}

func readProtectedFile(name string, policy protectedFilePolicy) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("文件路径为空")
	}
	resolvedName, err := filepath.EvalSymlinks(name)
	if err != nil {
		return nil, fmt.Errorf("解析文件失败: %w", err)
	}
	if err = validateProtectedDirectory(filepath.Dir(resolvedName)); err != nil {
		return nil, fmt.Errorf("文件所在目录不安全: %w", err)
	}
	file, err := os.Open(resolvedName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s文件不是普通文件", policy.description)
	}
	if info.Size() <= 0 || info.Size() > maximumProtectedFileSize {
		return nil, fmt.Errorf("%s文件大小非法", policy.description)
	}
	if info.Mode().Perm()&policy.forbiddenPermissions != 0 {
		return nil, fmt.Errorf("%s文件权限不安全", policy.description)
	}
	content, err := io.ReadAll(io.LimitReader(file, maximumProtectedFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || len(content) > maximumProtectedFileSize {
		return nil, fmt.Errorf("%s文件大小非法", policy.description)
	}
	return content, nil
}

func validateProtectedDirectory(name string) error {
	info, err := os.Stat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("不是目录")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("目录不能被组或其他用户写入")
	}
	return nil
}
