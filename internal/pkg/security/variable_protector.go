// Package security 定义敏感变量的保护边界。
package security

import (
	"fmt"

	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/Duke1616/etask/pkg/cryptox"
)

// VariableEncrypter 定义变量写入持久化介质时的加密能力。
type VariableEncrypter interface {
	// EncryptVariables 加密变量集合中的敏感值，并返回新的变量集合。
	EncryptVariables([]variable.Item) ([]variable.Item, error)
}

// VariableDecrypter 定义变量离开持久化介质时的解密能力。
type VariableDecrypter interface {
	// DecryptVariables 解密变量集合中的敏感值，并返回新的变量集合。
	DecryptVariables([]variable.Item) ([]variable.Item, error)
}

// VariableCipher 组合变量持久化所需的加密和解密能力。
// 仓储层只依赖此接口，不接触对外展示的脱敏能力。
type VariableCipher interface {
	VariableEncrypter
	VariableDecrypter
}

// VariableMasker 定义变量对外展示时的脱敏能力。
type VariableMasker interface {
	// MaskVariables 替换变量集合中的敏感值，过程中不会尝试解密。
	MaskVariables([]variable.Item) []variable.Item
}

// VariableProtector 组合变量的加密、解密和脱敏能力。
// 需要单一能力的调用方应依赖上面的细粒度接口，避免不必要的耦合。
type VariableProtector interface {
	VariableCipher
	VariableMasker
}

type variableProtector struct {
	valueProtector cryptox.ValueProtector
}

// NewVariableProtector 创建默认的变量保护器。
func NewVariableProtector(valueProtector cryptox.ValueProtector) VariableProtector {
	return &variableProtector{valueProtector: valueProtector}
}

// NewVariableMasker 创建只具备脱敏能力的变量处理器，适用于接口展示等场景。
func NewVariableMasker() VariableMasker {
	return &variableProtector{}
}

func (p *variableProtector) protectItem(item variable.Item) (variable.Item, error) {
	if !item.Secret || item.Value == "" {
		return item, nil
	}
	if p.valueProtector == nil {
		return variable.Item{}, fmt.Errorf("敏感变量 %q 缺少保护器", item.Key)
	}
	value, err := p.valueProtector.Encrypt(item.Value)
	if err != nil {
		return variable.Item{}, fmt.Errorf("加密变量 %q 失败: %w", item.Key, err)
	}
	item.Value = value
	return item, nil
}

func (p *variableProtector) revealItem(item variable.Item) (variable.Item, error) {
	if !item.Secret || item.Value == "" {
		return item, nil
	}
	if p.valueProtector == nil {
		return variable.Item{}, fmt.Errorf("敏感变量 %q 缺少保护器", item.Key)
	}
	if decoder, ok := p.valueProtector.(cryptox.CiphertextDecryptor); ok {
		value, err := decoder.DecryptCiphertext(item.Value)
		if err != nil {
			return variable.Item{}, fmt.Errorf("敏感变量 %q 不是合法密文: %w", item.Key, err)
		}
		item.Value = value
		return item, nil
	}
	if len(item.Value) < len(cryptox.EncryptedPrefix) || item.Value[:len(cryptox.EncryptedPrefix)] != cryptox.EncryptedPrefix {
		return variable.Item{}, fmt.Errorf("敏感变量 %q 不是合法密文", item.Key)
	}
	value, err := p.valueProtector.Decrypt(item.Value)
	if err != nil {
		return variable.Item{}, fmt.Errorf("解密变量 %q 失败: %w", item.Key, err)
	}
	item.Value = value
	return item, nil
}

func (p *variableProtector) maskItem(item variable.Item) variable.Item {
	if item.Secret {
		item.Value = cryptox.DefaultMask
	}
	return item
}

func (p *variableProtector) EncryptVariables(items []variable.Item) ([]variable.Item, error) {
	result := append([]variable.Item(nil), items...)
	for index := range result {
		item, err := p.protectItem(result[index])
		if err != nil {
			return nil, err
		}
		result[index] = item
	}
	return result, nil
}

func (p *variableProtector) DecryptVariables(items []variable.Item) ([]variable.Item, error) {
	result := append([]variable.Item(nil), items...)
	for index := range result {
		item, err := p.revealItem(result[index])
		if err != nil {
			return nil, err
		}
		result[index] = item
	}
	return result, nil
}

func (p *variableProtector) MaskVariables(items []variable.Item) []variable.Item {
	result := append([]variable.Item(nil), items...)
	for index := range result {
		result[index] = p.maskItem(result[index])
	}
	return result
}
