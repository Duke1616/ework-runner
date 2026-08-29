package repository

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/pkg/security"
	"github.com/Duke1616/etask/internal/repository/dao"
)

type VariableRepository interface {
	// CreateGlobalVariable 创建全局变量。
	CreateGlobalVariable(ctx context.Context, variable domain.Variable) (int64, error)
	// FindGlobalVariable 根据主键 ID 查询全局变量。
	FindGlobalVariable(ctx context.Context, id int64) (domain.Variable, error)
	// UpdateGlobalVariable 更新全局变量。
	UpdateGlobalVariable(ctx context.Context, variable domain.Variable) (int64, error)
	// DeleteGlobalVariable 根据主键 ID 删除全局变量。
	DeleteGlobalVariable(ctx context.Context, id int64) (int64, error)
	// ListGlobalVariables 分页查询全局变量。
	ListGlobalVariables(ctx context.Context, offset, limit int64, keyword string) ([]domain.Variable, error)
	// CountGlobalVariables 统计全局变量数量。
	CountGlobalVariables(ctx context.Context, keyword string) (int64, error)
}

type variableRepository struct {
	dao       dao.VariableDAO
	protector security.VariableCipher
}

func NewVariableRepository(variableDAO dao.VariableDAO, protector security.VariableCipher) VariableRepository {
	return &variableRepository{dao: variableDAO, protector: protector}
}

// CreateGlobalVariable 创建全局变量。
func (repo *variableRepository) CreateGlobalVariable(ctx context.Context, variable domain.Variable) (int64, error) {
	variable.MarkGlobal()
	if err := variable.Validate(); err != nil {
		return 0, err
	}
	po, err := repo.toDAO(variable)
	if err != nil {
		return 0, err
	}
	return repo.dao.Create(ctx, po)
}

// FindGlobalVariable 根据主键 ID 查询全局变量。
func (repo *variableRepository) FindGlobalVariable(ctx context.Context, id int64) (domain.Variable, error) {
	if err := domain.ValidateVariableID(id); err != nil {
		return domain.Variable{}, err
	}
	variable, err := repo.dao.FindByID(ctx, id)
	if err != nil {
		return domain.Variable{}, err
	}
	res, err := repo.toDomain(variable)
	if err != nil {
		return domain.Variable{}, err
	}
	if err = res.ValidateGlobalScope(); err != nil {
		return domain.Variable{}, err
	}
	return res, nil
}

// UpdateGlobalVariable 更新全局变量。
func (repo *variableRepository) UpdateGlobalVariable(ctx context.Context, variable domain.Variable) (int64, error) {
	if err := variable.ValidateID(); err != nil {
		return 0, err
	}
	variable.MarkGlobal()
	if err := variable.Validate(); err != nil {
		return 0, err
	}
	po, err := repo.toDAO(variable)
	if err != nil {
		return 0, err
	}
	return repo.dao.Update(ctx, po)
}

// DeleteGlobalVariable 根据主键 ID 删除全局变量。
func (repo *variableRepository) DeleteGlobalVariable(ctx context.Context, id int64) (int64, error) {
	if err := domain.ValidateVariableID(id); err != nil {
		return 0, err
	}
	return repo.dao.DeleteByIDAndScope(ctx, id, domain.VariableScopeGlobal.String(), domain.VariableGlobalTarget)
}

// ListGlobalVariables 分页查询全局变量。
func (repo *variableRepository) ListGlobalVariables(ctx context.Context, offset, limit int64, keyword string) ([]domain.Variable, error) {
	variables, err := repo.dao.ListByScopePage(ctx, domain.VariableScopeGlobal.String(), domain.VariableGlobalTarget, offset, limit, keyword)
	if err != nil {
		return nil, err
	}
	return repo.toDomains(variables)
}

// CountGlobalVariables 统计全局变量数量。
func (repo *variableRepository) CountGlobalVariables(ctx context.Context, keyword string) (int64, error) {
	return repo.dao.CountByScope(ctx, domain.VariableScopeGlobal.String(), domain.VariableGlobalTarget, keyword)
}

func (repo *variableRepository) toDAO(src domain.Variable) (dao.Variable, error) {
	value, err := repo.encryptSecretValue(src.Key, src.Value, src.Secret)
	if err != nil {
		return dao.Variable{}, err
	}
	return dao.Variable{
		ID:       src.ID,
		TenantID: src.TenantID,
		Scope:    src.Scope.String(),
		TargetID: src.TargetID,
		Key:      src.Key,
		Value:    value,
		Secret:   src.Secret,
		CTime:    src.CTime,
		UTime:    src.UTime,
	}, nil
}

func (repo *variableRepository) toDomain(src dao.Variable) (domain.Variable, error) {
	value, err := repo.decryptSecretValue(src.Key, src.Value, src.Secret)
	if err != nil {
		return domain.Variable{}, err
	}
	return domain.Variable{
		ID:       src.ID,
		TenantID: src.TenantID,
		Scope:    domain.VariableScope(src.Scope),
		TargetID: src.TargetID,
		Key:      src.Key,
		Value:    value,
		Secret:   src.Secret,
		CTime:    src.CTime,
		UTime:    src.UTime,
	}, nil
}

func (repo *variableRepository) toDomains(variables []dao.Variable) ([]domain.Variable, error) {
	res := make([]domain.Variable, 0, len(variables))
	for _, variable := range variables {
		item, err := repo.toDomain(variable)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	return res, nil
}

func (repo *variableRepository) encryptSecretValue(key, value string, secret bool) (string, error) {
	items, err := repo.protector.EncryptVariables([]domain.RunnerVariable{{Key: key, Value: value, Secret: secret}})
	if err != nil {
		return "", err
	}
	return items[0].Value, nil
}

func (repo *variableRepository) decryptSecretValue(key, value string, secret bool) (string, error) {
	items, err := repo.protector.DecryptVariables([]domain.RunnerVariable{{Key: key, Value: value, Secret: secret}})
	if err != nil {
		return "", err
	}
	return items[0].Value, nil
}
