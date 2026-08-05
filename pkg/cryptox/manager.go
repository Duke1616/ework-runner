package cryptox

import (
	"fmt"
	"strings"
)

type MigrationHandler func(oldEncrypted, newEncrypted string)

// CryptoManager 管理多版本算法，并兼容没有版本前缀的历史密文。
type CryptoManager struct {
	algorithms  map[string]Crypto
	defaultVer  string
	legacyVer   string
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
	m.legacyVer = version
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
	if !strings.HasPrefix(encryptedText, EncryptedPrefix) {
		if value, ok := m.tryLegacyDecrypt(encryptedText); ok {
			return value, nil
		}
		return encryptedText, nil
	}
	trimmed := strings.TrimPrefix(encryptedText, EncryptedPrefix)
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted format")
	}
	algorithm, ok := m.algorithms[parts[0]]
	if !ok {
		return "", fmt.Errorf("unsupported encryption version: %s", parts[0])
	}
	return algorithm.Decrypt(parts[1])
}

func (m *CryptoManager) tryLegacyDecrypt(encryptedText string) (string, bool) {
	algorithm, ok := m.algorithms[m.legacyVer]
	if m.legacyVer == "" || !ok {
		return "", false
	}
	value, err := algorithm.Decrypt(encryptedText)
	if err != nil {
		return "", false
	}
	if m.onMigration != nil {
		if migrated, migrateErr := m.Encrypt(value); migrateErr == nil {
			m.onMigration(encryptedText, migrated)
		}
	}
	return value, true
}
