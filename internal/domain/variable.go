package domain

import (
	"fmt"

	"github.com/Duke1616/etask/internal/errs"
)

type VariableScope string

const (
	VariableScopeGlobal  VariableScope = "GLOBAL"
	VariableScopeRunner  VariableScope = "RUNNER"
	VariableGlobalTarget int64         = 0
)

func (s VariableScope) String() string {
	return string(s)
}

// Valid 判断变量作用域是否合法。
func (s VariableScope) Valid() bool {
	return s == VariableScopeGlobal || s == VariableScopeRunner
}

type Variable struct {
	ID       int64
	TenantID int64
	Scope    VariableScope
	TargetID int64
	Key      string
	Value    string
	Secret   bool
	CTime    int64
	UTime    int64
}

// MarkGlobal 将变量标记为全局变量。
func (v *Variable) MarkGlobal() {
	v.Scope = VariableScopeGlobal
	v.TargetID = VariableGlobalTarget
}

// IsGlobal 判断变量是否属于全局作用域。
func (v *Variable) IsGlobal() bool {
	return v.Scope == VariableScopeGlobal && v.TargetID == VariableGlobalTarget
}

// ValidateGlobalScope 校验变量是否属于全局作用域。
func (v *Variable) ValidateGlobalScope() error {
	if !v.IsGlobal() {
		return fmt.Errorf("%w: variable is not global", errs.ErrInvalidParameter)
	}
	return nil
}

// KeepSecretValueFrom 在敏感变量更新未传值时使用旧变量值。
func (v *Variable) KeepSecretValueFrom(old Variable) {
	if v.Secret && v.Value == "" {
		v.Value = old.Value
	}
}

// ValidateID 校验变量主键 ID。
func (v *Variable) ValidateID() error {
	return ValidateVariableID(v.ID)
}

// ValidateVariableID 校验变量主键 ID。
func ValidateVariableID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("%w: id = %d", errs.ErrInvalidParameter, id)
	}
	return nil
}

// Validate 校验变量持久化前的必要字段。
func (v *Variable) Validate() error {
	if v.Scope == "" {
		return fmt.Errorf("%w: scope is empty", errs.ErrInvalidParameter)
	}
	if !v.Scope.Valid() {
		return fmt.Errorf("%w: unsupported scope = %s", errs.ErrInvalidParameter, v.Scope)
	}
	if v.Scope == VariableScopeGlobal && v.TargetID != VariableGlobalTarget {
		return fmt.Errorf("%w: global target_id is invalid", errs.ErrInvalidParameter)
	}
	if v.Scope == VariableScopeRunner && v.TargetID <= 0 {
		return fmt.Errorf("%w: runner target_id is invalid", errs.ErrInvalidParameter)
	}
	if v.Key == "" {
		return fmt.Errorf("%w: key is empty", errs.ErrInvalidParameter)
	}
	return nil
}
