package cryptox

// EncryptedPrefix 标识带版本的密文。
const EncryptedPrefix = "ENC:"

// Crypto 定义字符串加解密能力。
type Crypto interface {
	Encrypt(plainText string) (string, error)
	Decrypt(encryptedText string) (string, error)
}
