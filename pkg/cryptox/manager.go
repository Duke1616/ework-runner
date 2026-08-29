package cryptox

import (
	"errors"
	"fmt"
	"strings"
)

type MigrationHandler func(oldEncrypted, newEncrypted string)

// ErrInvalidCiphertext 表示输入既不是当前格式密文，也不是已注册的历史密文。
var ErrInvalidCiphertext = errors.New("invalid ciphertext")

// CryptoManager 管理多版本算法，并兼容没有版本前缀的历史密文。
type CryptoManager struct {
	algorithms  map[string]Crypto
	defaultVer  string
	legacyVers  []string
	onMigration MigrationHandler
}

func NewCryptoManager(defaultVersion string) *CryptoManager {
	return &CryptoManager{algorithms: make(map[string]Crypto), defaultVer: defaultVersion}
}

func (m *CryptoManager) Register(version string, algorithm Crypto) *CryptoManager {
	m.algorithms[version] = algorithm
	return m
}

func (m *CryptoManager) WithLegacyAlgo(version string) *CryptoManager {
	if version != "" {
		for _, registered := range m.legacyVers {
			if registered == version {
				return m
			}
		}
		m.legacyVers = append(m.legacyVers, version)
	}
	return m
}

// WithLegacyAlgos 注册没有版本前缀的历史算法，按传入顺序依次尝试。
func (m *CryptoManager) WithLegacyAlgos(versions ...string) *CryptoManager {
	m.legacyVers = append([]string(nil), versions...)
	return m
}

func (m *CryptoManager) WithMigrationHandler(handler MigrationHandler) *CryptoManager {
	m.onMigration = handler
	return m
}

func (m *CryptoManager) Encrypt(plainText string) (string, error) {
	algorithm, ok := m.algorithms[m.defaultVer]
	if !ok {
		return "", fmt.Errorf("no default algorithm registered")
	}
	encrypted, err := algorithm.Encrypt(plainText)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s:%s", EncryptedPrefix, m.defaultVer, encrypted), nil
}

func (m *CryptoManager) Decrypt(encryptedText string) (string, error) {
	value, err := m.DecryptCiphertext(encryptedText)
	if err != nil {
		// 保留 Crypto 接口对普通字符串的历史兼容行为；持久化敏感变量由 security 包
		// 调用 DecryptCiphertext 严格校验，不能通过这里的宽松行为绕过保护边界。
		if !strings.HasPrefix(encryptedText, EncryptedPrefix) {
			return encryptedText, nil
		}
	}
	return value, err
}

// DecryptCiphertext 根据密文格式选择对应算法。
// 带 ENC:版本: 前缀的数据按指定版本解密；无前缀数据按历史算法顺序尝试。
// 无法由任何算法解密时返回错误，调用方不得将其当作合法密文。
func (m *CryptoManager) DecryptCiphertext(encryptedText string) (string, error) {
	if !strings.HasPrefix(encryptedText, EncryptedPrefix) {
		if value, ok := m.tryLegacyDecrypt(encryptedText); ok {
			return value, nil
		}
		return "", ErrInvalidCiphertext
	}
	trimmed := strings.TrimPrefix(encryptedText, EncryptedPrefix)
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("%w: invalid encrypted format", ErrInvalidCiphertext)
	}
	algorithm, ok := m.algorithms[parts[0]]
	if !ok {
		return "", fmt.Errorf("unsupported encryption version: %s", parts[0])
	}
	value, err := algorithm.Decrypt(parts[1])
	if err != nil {
		return "", err
	}
	return value, nil
}

func (m *CryptoManager) tryLegacyDecrypt(encryptedText string) (string, bool) {
	for _, legacyVer := range m.legacyVers {
		algorithm, ok := m.algorithms[legacyVer]
		if legacyVer == "" || !ok {
			continue
		}
		value, err := algorithm.Decrypt(encryptedText)
		if err != nil {
			continue
		}
		if m.onMigration != nil {
			if migrated, migrateErr := m.Encrypt(value); migrateErr == nil {
				m.onMigration(encryptedText, migrated)
			}
		}
		return value, true
	}
	return "", false
}
