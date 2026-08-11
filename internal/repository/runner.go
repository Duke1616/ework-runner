package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/Duke1616/etask/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

// ErrRunnerNotFound 表示执行单元不存在。
var ErrRunnerNotFound = gorm.ErrRecordNotFound

//go:generate go tool mockgen -source=./runner.go -package=repositorymocks -destination=./mocks/runner.mock.go -typed

// RunnerRepository 定义执行单元的领域仓储操作。
type RunnerRepository interface {
	// Create 保存执行单元。
	Create(ctx context.Context, req domain.Runner) (int64, error)
	// Update 更新执行单元可变字段。
	Update(ctx context.Context, req domain.Runner) (int64, error)
	// Delete 根据主键 ID 删除执行单元。
	Delete(ctx context.Context, id int64) (int64, error)
	// FindByID 根据主键 ID 加载执行单元。
	FindByID(ctx context.Context, id int64) (domain.Runner, error)
	// FindForExecution 加载执行单元及其全局和私有有效变量。
	FindForExecution(ctx context.Context, id int64) (domain.RunnerExecutionSpec, error)
	// List 分页查询执行单元。
	List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, error)
	// Count 统计匹配条件的执行单元总数。
	Count(ctx context.Context, keyword, kind string) (int64, error)
	// ListByCodebookID 查询绑定指定脚本模板 ID 的执行单元及其有效变量。
	ListByCodebookID(ctx context.Context, codebookID int64) ([]domain.RunnerExecutionSpec, error)
	// ListExcludeCodebookID 查询未绑定指定脚本模板 ID 的执行单元。
	ListExcludeCodebookID(ctx context.Context, offset, limit int64, codebookID int64, keyword, kind string) ([]domain.Runner, error)
	// CountExcludeCodebookID 统计未绑定指定脚本模板 ID 的执行单元数量。
	CountExcludeCodebookID(ctx context.Context, codebookID int64, keyword, kind string) (int64, error)
	// ListByCodebookIDs 查询绑定任一脚本模板 ID 的执行单元。
	ListByCodebookIDs(ctx context.Context, codebookIDs []int64) ([]domain.Runner, error)
	// ListByIDs 根据 ID 列表批量查询执行单元。
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Runner, error)
	// ListMergedVariables 获取执行单元变量，私有变量覆盖全局变量。
	ListMergedVariables(ctx context.Context, runnerID int64) ([]domain.RunnerVariable, error)
}

type runnerRepository struct {
	runnerDAO   dao.RunnerDAO
	variableDAO dao.VariableDAO
	crypto      cryptox.Crypto
}

// NewRunnerRepository 创建基于 RunnerDAO 的执行单元仓储。
func NewRunnerRepository(runnerDAO dao.RunnerDAO, variableDAO dao.VariableDAO, crypto cryptox.Crypto) RunnerRepository {
	return &runnerRepository{runnerDAO: runnerDAO, variableDAO: variableDAO, crypto: crypto}
}

// Create 保存执行单元。
func (repo *runnerRepository) Create(ctx context.Context, req domain.Runner) (int64, error) {
	variables, err := repo.toVariables(req.ID, req.Variables)
	if err != nil {
		return 0, err
	}
	return repo.runnerDAO.CreateWithVariables(ctx, repo.toEntity(req), variables)
}

// Update 更新执行单元可变字段。
func (repo *runnerRepository) Update(ctx context.Context, req domain.Runner) (int64, error) {
	variables, err := repo.toVariables(req.ID, req.Variables)
	if err != nil {
		return 0, err
	}
	return repo.runnerDAO.UpdateWithVariables(ctx, repo.toEntity(req), variables)
}

// Delete 根据主键 ID 删除执行单元。
func (repo *runnerRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return repo.runnerDAO.DeleteWithVariables(ctx, id)
}

// FindByID 根据主键 ID 加载执行单元。
func (repo *runnerRepository) FindByID(ctx context.Context, id int64) (domain.Runner, error) {
	r, err := repo.runnerDAO.FindByID(ctx, id)
	if err != nil {
		return domain.Runner{}, err
	}
	res := repo.toDomain(r)
	res.Variables, err = repo.listRunnerVariables(ctx, id)
	if err != nil {
		return domain.Runner{}, err
	}
	return res, nil
}

func (repo *runnerRepository) FindForExecution(ctx context.Context, id int64) (domain.RunnerExecutionSpec, error) {
	r, err := repo.runnerDAO.FindByID(ctx, id)
	if err != nil {
		return domain.RunnerExecutionSpec{}, err
	}
	variables, err := repo.ListMergedVariables(ctx, id)
	if err != nil {
		return domain.RunnerExecutionSpec{}, err
	}
	return domain.RunnerExecutionSpec{Runner: repo.toDomain(r), Variables: variables}, nil
}

// List 分页查询执行单元。
func (repo *runnerRepository) List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, error) {
	rs, err := repo.runnerDAO.List(ctx, offset, limit, keyword, kind)
	if err != nil {
		return nil, err
	}
	return slice.Map(rs, func(_ int, src dao.Runner) domain.Runner {
		return repo.toDomain(src)
	}), nil
}

// Count 统计匹配条件的执行单元总数。
func (repo *runnerRepository) Count(ctx context.Context, keyword, kind string) (int64, error) {
	return repo.runnerDAO.Count(ctx, keyword, kind)
}

// ListByCodebookID 查询绑定指定脚本模板 ID 的全部执行单元。
func (repo *runnerRepository) ListByCodebookID(ctx context.Context, codebookID int64) ([]domain.RunnerExecutionSpec, error) {
	rs, err := repo.runnerDAO.ListByCodebookID(ctx, codebookID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.RunnerExecutionSpec, 0, len(rs))
	for _, src := range rs {
		variables, err := repo.ListMergedVariables(ctx, src.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, domain.RunnerExecutionSpec{
			Runner: repo.toDomain(src), Variables: variables,
		})
	}
	return result, nil
}

// ListExcludeCodebookID 查询未绑定指定脚本模板 ID 的执行单元。
func (repo *runnerRepository) ListExcludeCodebookID(ctx context.Context, offset, limit int64, codebookID int64, keyword, kind string) ([]domain.Runner, error) {
	rs, err := repo.runnerDAO.ListExcludeCodebookID(ctx, offset, limit, codebookID, keyword, kind)
	if err != nil {
		return nil, err
	}
	return slice.Map(rs, func(_ int, src dao.Runner) domain.Runner {
		return repo.toDomain(src)
	}), nil
}

// CountExcludeCodebookID 统计未绑定指定脚本模板 ID 的执行单元数量。
func (repo *runnerRepository) CountExcludeCodebookID(ctx context.Context, codebookID int64, keyword, kind string) (int64, error) {
	return repo.runnerDAO.CountExcludeCodebookID(ctx, codebookID, keyword, kind)
}

// ListByCodebookIDs 查询绑定任一脚本模板 ID 的执行单元。
func (repo *runnerRepository) ListByCodebookIDs(ctx context.Context, codebookIDs []int64) ([]domain.Runner, error) {
	rs, err := repo.runnerDAO.ListByCodebookIDs(ctx, codebookIDs)
	if err != nil {
		return nil, err
	}
	return slice.Map(rs, func(_ int, src dao.Runner) domain.Runner {
		return repo.toDomain(src)
	}), nil
}

// ListByIDs 根据 ID 列表批量查询执行单元。
func (repo *runnerRepository) ListByIDs(ctx context.Context, ids []int64) ([]domain.Runner, error) {
	rs, err := repo.runnerDAO.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return slice.Map(rs, func(_ int, src dao.Runner) domain.Runner {
		return repo.toDomain(src)
	}), nil
}

func (repo *runnerRepository) ListMergedVariables(ctx context.Context, runnerID int64) ([]domain.RunnerVariable, error) {
	variables, err := repo.variableDAO.ListGlobalAndRunner(ctx, runnerID)
	if err != nil {
		return nil, err
	}
	return repo.toRunnerVariables(mergeVariablesByKey(variables))
}

func mergeVariablesByKey(variables []dao.Variable) []dao.Variable {
	merged := make([]dao.Variable, 0, len(variables))
	indexes := make(map[string]int, len(variables))
	for _, variable := range variables {
		idx, ok := indexes[variable.Key]
		if ok {
			merged[idx] = variable
			continue
		}
		indexes[variable.Key] = len(merged)
		merged = append(merged, variable)
	}
	return merged
}

func (repo *runnerRepository) toEntity(req domain.Runner) dao.Runner {
	return dao.Runner{
		ID:                req.ID,
		TenantID:          req.TenantID,
		Name:              req.Name,
		CodebookID:        req.CodebookID,
		ProgramKind:       string(req.ProgramKind),
		CodebookSecret:    req.CodebookSecret,
		Kind:              req.Kind.String(),
		Target:            req.Target,
		Handler:           req.Handler,
		Tags:              sqlx.JSONColumn[[]string]{Val: req.Tags, Valid: true},
		Action:            req.Action.Uint8(),
		Desc:              req.Desc,
		ParameterDefaults: sqlx.JSONColumn[map[string]json.RawMessage]{Val: req.ParameterDefaults, Valid: req.ParameterDefaults != nil},
		CTime:             req.CTime,
		UTime:             req.UTime,
	}
}

func (repo *runnerRepository) toDomain(req dao.Runner) domain.Runner {
	r := domain.Runner{
		ID:                req.ID,
		TenantID:          req.TenantID,
		Name:              req.Name,
		CodebookID:        req.CodebookID,
		ProgramKind:       domain.ProgramKind(req.ProgramKind),
		CodebookSecret:    req.CodebookSecret,
		Kind:              domain.RunnerKind(req.Kind),
		Target:            req.Target,
		Handler:           req.Handler,
		Tags:              req.Tags.Val,
		Action:            domain.RunnerAction(req.Action),
		Desc:              req.Desc,
		ParameterDefaults: req.ParameterDefaults.Val,
		CTime:             req.CTime,
		UTime:             req.UTime,
	}
	if req.Kind == "" {
		r.Kind = domain.RunnerKindKafka
	}
	return r
}

func (repo *runnerRepository) toVariables(targetID int64, variables []domain.RunnerVariable) ([]dao.Variable, error) {
	merged := make(map[string]domain.RunnerVariable, len(variables))
	keys := make([]string, 0, len(variables))
	for _, src := range variables {
		if src.Key == "" {
			continue
		}
		if _, ok := merged[src.Key]; !ok {
			keys = append(keys, src.Key)
		}
		merged[src.Key] = src
	}
	res := make([]dao.Variable, 0, len(keys))
	for _, key := range keys {
		src := merged[key]
		value := src.Value
		if src.Secret && value != "" {
			encVal, err := repo.crypto.Encrypt(value)
			if err != nil {
				return nil, fmt.Errorf("encrypt runner variable %q failed: %w", src.Key, err)
			}
			value = encVal
		}
		res = append(res, dao.Variable{
			Scope:    domain.VariableScopeRunner.String(),
			TargetID: targetID,
			Key:      src.Key,
			Value:    value,
			Secret:   src.Secret,
		})
	}
	return res, nil
}

func (repo *runnerRepository) listRunnerVariables(ctx context.Context, runnerID int64) ([]domain.RunnerVariable, error) {
	variables, err := repo.variableDAO.ListByScope(ctx, domain.VariableScopeRunner.String(), runnerID)
	if err != nil {
		return nil, err
	}
	return repo.toRunnerVariables(variables)
}

func (repo *runnerRepository) toRunnerVariables(variables []dao.Variable) ([]domain.RunnerVariable, error) {
	res := make([]domain.RunnerVariable, 0, len(variables))
	for _, src := range variables {
		value := src.Value
		if src.Secret && value != "" {
			decVal, err := repo.crypto.Decrypt(value)
			if err != nil {
				return nil, fmt.Errorf("decrypt runner variable %q failed: %w", src.Key, err)
			}
			value = decVal
		}
		res = append(res, domain.RunnerVariable{
			Key:    src.Key,
			Value:  value,
			Secret: src.Secret,
		})
	}
	return res, nil
}
