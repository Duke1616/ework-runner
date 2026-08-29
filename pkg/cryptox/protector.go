package cryptox

import "strings"

// Masker 提供敏感值的安全展示形式。
type Masker interface {
	Mask(value string) string
}

// ValueProtector 统一封装敏感值在存储、运行时和展示边界上的处理。
// Encrypt 和 Decrypt 用于持久化与运行时边界，Mask 用于接口等展示场景。
type ValueProtector interface {
	Crypto
	Masker
}

// CiphertextDecryptor 只接受可确认的密文并返回解密后的值。
// 该接口会兼容没有 ENC:版本: 前缀的历史密文，但不会把数据库明文当成合法密文。
type CiphertextDecryptor interface {
	DecryptCiphertext(encryptedText string) (string, error)
}

// DefaultMask 是敏感值对外展示时使用的统一占位文本。
// 它只表达“该值已被隐藏”，不会泄露原始值的长度或内容。
const DefaultMask = "[已脱敏]"

type valueProtector struct {
	Crypto
	mask string
}

// NewValueProtector 为已有加密组件增加统一的展示掩码。
func NewValueProtector(cipher Crypto) ValueProtector {
	if cipher == nil {
		return nil
	}
	if protector, ok := cipher.(ValueProtector); ok {
		return protector
	}
	return &valueProtector{Crypto: cipher, mask: DefaultMask}
}

func (p *valueProtector) Mask(string) string {
	return p.mask
}

// DecryptCiphertext 将严格密文解密转发给底层加密管理器。
func (p *valueProtector) DecryptCiphertext(encryptedText string) (string, error) {
	decoder, ok := p.Crypto.(CiphertextDecryptor)
	if !ok {
		if !strings.HasPrefix(encryptedText, EncryptedPrefix) {
			return "", ErrInvalidCiphertext
		}
		return p.Crypto.Decrypt(encryptedText)
	}
	return decoder.DecryptCiphertext(encryptedText)
}

// MaskValue 用于不需要解密、只需要生成展示结果的投影边界。
func MaskValue(string) string { return DefaultMask }
