package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
)

func TestBindingServiceBindNormalizesWildcard(t *testing.T) {
	bindingRepo := newFakeBindingRepo()
	svc := NewBindingService(
		&fakePoolRepo{
			pools: map[string]domain.ExecutionPool{
				"default": enabledPool("default"),
			},
		},
		bindingRepo,
	)

	err := svc.Bind(context.Background(), BindingRequest{
		PoolName:    " default ",
		HandlerName: "*",
		Desc:        " whole pool ",
	})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}

	binding, ok := bindingRepo.bindings[bindingKey{poolName: "default", handlerName: ""}]
	if !ok {
		t.Fatal("wildcard binding was not stored")
	}
	if binding.Status != domain.ExecutionPoolBindingStatusEnabled {
		t.Fatalf("binding.Status = %s, want %s", binding.Status, domain.ExecutionPoolBindingStatusEnabled)
	}
	if binding.Desc != "whole pool" {
		t.Fatalf("binding.Desc = %q, want %q", binding.Desc, "whole pool")
	}
}

func TestBindingServiceBindManyCreatesHandlers(t *testing.T) {
	bindingRepo := newFakeBindingRepo()
	svc := NewBindingService(
		&fakePoolRepo{
			pools: map[string]domain.ExecutionPool{
				"default": {
					Name:   "default",
					Status: domain.ExecutionPoolStatusEnabled,
					Metadata: map[string]string{
						"supported_handlers": `[{"name":"run"},{"name":"stop"}]`,
					},
				},
			},
		},
		bindingRepo,
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"run", "stop", "run"},
		Desc:         " handlers ",
	})
	if err != nil {
		t.Fatalf("BindMany() error = %v", err)
	}

	if len(bindingRepo.bindings) != 2 {
		t.Fatalf("len(bindings) = %d, want 2", len(bindingRepo.bindings))
	}
	for _, handlerName := range []string{"run", "stop"} {
		binding, ok := bindingRepo.bindings[bindingKey{poolName: "default", handlerName: handlerName}]
		if !ok {
			t.Fatalf("handler %s was not stored", handlerName)
		}
		if binding.Desc != "handlers" {
			t.Fatalf("binding.Desc = %q, want %q", binding.Desc, "handlers")
		}
	}
}

func TestBindingServiceBindManyRejectsWildcardMixedWithHandlers(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		nil,
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"*", "run"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyRejectsExistingWildcard(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: ""}: enabledBinding("default", ""),
		},
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"run"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyRejectsWildcardWhenHandlersExist(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: "run"}: enabledBinding("default", "run"),
		},
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"*"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyRejectsDuplicateHandler(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: "run"}: disabledBinding("default", "run"),
		},
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"run"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyRejectsUnsupportedHandler(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{
			"default": {
				Name:   "default",
				Status: domain.ExecutionPoolStatusEnabled,
				Metadata: map[string]string{
					"supported_handlers": `[{"name":"run"}]`,
				},
			},
		},
		nil,
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"stop"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyReturnsHandlerMetadataError(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{
			"default": {
				Name:   "default",
				Status: domain.ExecutionPoolStatusEnabled,
				Metadata: map[string]string{
					"supported_handlers": "{invalid-json",
				},
			},
		},
		nil,
	)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName: "default", HandlerNames: []string{"run"},
	})
	if err == nil {
		t.Fatal("BindMany() error = nil, want invalid metadata error")
	}
}

func TestBindingServiceBindManyRejectsDedicatedPoolOccupiedByAnotherTenant(t *testing.T) {
	bindingRepo := newFakeBindingRepo()
	bindingRepo.occupiedPoolNames = map[string]bool{"dedicated": true}
	svc := NewBindingService(&fakePoolRepo{pools: map[string]domain.ExecutionPool{
		"dedicated": {
			Name:           "dedicated",
			Status:         domain.ExecutionPoolStatusEnabled,
			IsolationLevel: domain.ExecutionPoolIsolationDedicated,
		},
	}}, bindingRepo)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "dedicated",
		HandlerNames: []string{"*"},
	})
	if !errors.Is(err, errs.ErrInvalidParameter) {
		t.Fatalf("BindMany() error = %v, want invalid parameter", err)
	}
}

func TestBindingServiceBindManyIsAtomic(t *testing.T) {
	bindingRepo := newFakeBindingRepo()
	bindingRepo.batchErr = errors.New("database unavailable")
	svc := NewBindingService(&fakePoolRepo{pools: map[string]domain.ExecutionPool{
		"default": enabledPool("default"),
	}}, bindingRepo)

	err := svc.BindMany(context.Background(), BindingManyRequest{
		PoolName:     "default",
		HandlerNames: []string{"run", "stop"},
	})
	if err == nil {
		t.Fatal("BindMany() error = nil, want error")
	}
	if len(bindingRepo.bindings) != 0 {
		t.Fatalf("len(bindings) = %d, want 0", len(bindingRepo.bindings))
	}
}

func TestBindingServiceIsAllowedByExactBinding(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: "run"}: enabledBinding("default", "run"),
		},
	)

	allowed, err := svc.IsAllowed(context.Background(), CheckBindingRequest{
		PoolName:    "default",
		HandlerName: "run",
	})
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if !allowed {
		t.Fatal("IsAllowed() = false, want true")
	}
}

func TestBindingServiceIsAllowedFallsBackToWildcard(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: ""}: enabledBinding("default", ""),
		},
	)

	allowed, err := svc.IsAllowed(context.Background(), CheckBindingRequest{
		PoolName:    "default",
		HandlerName: "run",
	})
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if !allowed {
		t.Fatal("IsAllowed() = false, want true")
	}
}

func TestBindingServiceIsAllowedExactDisabledOverridesWildcard(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{"default": enabledPool("default")},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: ""}:    enabledBinding("default", ""),
			{poolName: "default", handlerName: "run"}: disabledBinding("default", "run"),
		},
	)

	allowed, err := svc.IsAllowed(context.Background(), CheckBindingRequest{
		PoolName:    "default",
		HandlerName: "run",
	})
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if allowed {
		t.Fatal("IsAllowed() = true, want false")
	}
}

func TestBindingServiceIsAllowedRejectsDisabledPool(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{
			"default": {
				Name:   "default",
				Status: domain.ExecutionPoolStatusDisabled,
			},
		},
		map[bindingKey]domain.ExecutionPoolBinding{
			{poolName: "default", handlerName: "run"}: enabledBinding("default", "run"),
		},
	)

	allowed, err := svc.IsAllowed(context.Background(), CheckBindingRequest{
		PoolName:    "default",
		HandlerName: "run",
	})
	if err != nil {
		t.Fatalf("IsAllowed() error = %v", err)
	}
	if allowed {
		t.Fatal("IsAllowed() = true, want false")
	}
}

func TestBindingServiceRuntimeOverridableParameterKeys(t *testing.T) {
	svc := newTestBindingService(
		map[string]domain.ExecutionPool{
			"default": {
				Name:   "default",
				Status: domain.ExecutionPoolStatusEnabled,
				Metadata: map[string]string{
					"supported_handlers": `[{
						"name":"ansible",
						"metadata":[
							{"key":"limit","runtime_overridable":true},
							{"key":"inventory","runtime_overridable":false}
						]
					}]`,
				},
			},
		},
		nil,
	)

	keys, err := svc.RuntimeOverridableParameterKeys(context.Background(), CheckBindingRequest{
		PoolName: "default", HandlerName: "ansible",
	})
	if err != nil {
		t.Fatalf("RuntimeOverridableParameterKeys() error = %v", err)
	}
	if _, ok := keys["limit"]; !ok {
		t.Fatal("RuntimeOverridableParameterKeys() missing limit")
	}
	if _, ok := keys["inventory"]; ok {
		t.Fatal("RuntimeOverridableParameterKeys() unexpectedly contains inventory")
	}
}

func TestCatalogServiceListAuthorizedPoolsFiltersByPoolState(t *testing.T) {
	poolRepo := &fakePoolRepo{
		pools: map[string]domain.ExecutionPool{
			"agent": {
				Name:   "agent",
				Kind:   domain.ExecutionPoolKindAgent,
				Status: domain.ExecutionPoolStatusEnabled,
			},
			"disabled": {
				Name:   "disabled",
				Kind:   domain.ExecutionPoolKindExecutor,
				Status: domain.ExecutionPoolStatusDisabled,
			},
			"executor": {
				Name:   "executor",
				Kind:   domain.ExecutionPoolKindExecutor,
				Status: domain.ExecutionPoolStatusEnabled,
			},
		},
	}
	bindingRepo := newFakeBindingRepo()
	bindingRepo.poolNames = []string{"agent", "disabled", "executor"}
	svc := NewCatalogService(poolRepo, bindingRepo, nil)

	page, err := svc.ListAuthorizedPools(context.Background(), CatalogListRequest{
		Kind:  domain.ExecutionPoolKindExecutor,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAuthorizedPools() error = %v", err)
	}
	if len(page.Pools) != 1 || page.Pools[0].Name != "executor" {
		t.Fatalf("page.Pools = %#v, want executor only", page.Pools)
	}
}

func newTestBindingService(
	pools map[string]domain.ExecutionPool,
	bindings map[bindingKey]domain.ExecutionPoolBinding,
) *testBindingServices {
	bindingRepo := newFakeBindingRepo()
	if bindings != nil {
		bindingRepo.bindings = bindings
	}
	poolRepo := &fakePoolRepo{pools: pools}
	return &testBindingServices{
		BindingService:          NewBindingService(poolRepo, bindingRepo),
		ExecutionPoolAuthorizer: NewExecutionPoolAuthorizer(poolRepo, bindingRepo),
		HandlerMetadataProvider: NewHandlerMetadataProvider(poolRepo),
	}
}

type testBindingServices struct {
	BindingService
	ExecutionPoolAuthorizer
	HandlerMetadataProvider
}

func enabledPool(name string) domain.ExecutionPool {
	return domain.ExecutionPool{
		Name:   name,
		Status: domain.ExecutionPoolStatusEnabled,
	}
}

func enabledBinding(poolName, handlerName string) domain.ExecutionPoolBinding {
	return domain.ExecutionPoolBinding{
		PoolName:    poolName,
		HandlerName: handlerName,
		Status:      domain.ExecutionPoolBindingStatusEnabled,
	}
}

func disabledBinding(poolName, handlerName string) domain.ExecutionPoolBinding {
	binding := enabledBinding(poolName, handlerName)
	binding.Status = domain.ExecutionPoolBindingStatusDisabled
	return binding
}

type fakePoolRepo struct {
	pools map[string]domain.ExecutionPool
}

func (f *fakePoolRepo) Upsert(context.Context, domain.ExecutionPool) error {
	return nil
}

func (f *fakePoolRepo) Disable(context.Context, string) error {
	return nil
}

func (f *fakePoolRepo) Find(_ context.Context, name string) (domain.ExecutionPool, error) {
	pool, ok := f.pools[name]
	if !ok {
		return domain.ExecutionPool{}, repository.ErrExecutionPoolNotFound
	}
	return pool, nil
}

func (f *fakePoolRepo) ListByKind(context.Context, domain.ExecutionPoolKind) ([]domain.ExecutionPool, error) {
	return nil, nil
}

func (f *fakePoolRepo) ListByNames(
	_ context.Context,
	names []string,
	kind domain.ExecutionPoolKind,
	status domain.ExecutionPoolStatus,
	keyword string,
) ([]domain.ExecutionPool, error) {
	res := make([]domain.ExecutionPool, 0, len(names))
	for _, name := range names {
		pool, ok := f.pools[name]
		if !ok {
			continue
		}
		if kind != "" && pool.Kind != kind {
			continue
		}
		if status != "" && pool.Status != status {
			continue
		}
		res = append(res, pool)
	}
	return res, nil
}

func (f *fakePoolRepo) List(
	_ context.Context,
	offset int64,
	limit int64,
	_ string,
	kind domain.ExecutionPoolKind,
	_ domain.ExecutionTransport,
	_ domain.ExecMode,
	status domain.ExecutionPoolStatus,
) ([]domain.ExecutionPool, error) {
	pools := make([]domain.ExecutionPool, 0, len(f.pools))
	for _, pool := range f.pools {
		if kind != "" && pool.Kind != kind {
			continue
		}
		if status != "" && pool.Status != status {
			continue
		}
		pools = append(pools, pool)
	}
	end := offset + limit
	if end > int64(len(pools)) {
		end = int64(len(pools))
	}
	if offset > int64(len(pools)) {
		return nil, nil
	}
	return pools[offset:end], nil
}

func (f *fakePoolRepo) Count(
	_ context.Context,
	_ string,
	kind domain.ExecutionPoolKind,
	_ domain.ExecutionTransport,
	_ domain.ExecMode,
	status domain.ExecutionPoolStatus,
) (int64, error) {
	pools, err := f.List(context.Background(), 0, int64(len(f.pools)), "", kind, "", "", status)
	return int64(len(pools)), err
}

type bindingKey struct {
	poolName    string
	handlerName string
}

type fakeBindingRepo struct {
	bindings          map[bindingKey]domain.ExecutionPoolBinding
	poolNames         []string
	occupiedPoolNames map[string]bool
	batchErr          error
}

func newFakeBindingRepo() *fakeBindingRepo {
	return &fakeBindingRepo{
		bindings: make(map[bindingKey]domain.ExecutionPoolBinding),
	}
}

func (f *fakeBindingRepo) Bind(_ context.Context, binding domain.ExecutionPoolBinding) error {
	f.bindings[bindingKey{
		poolName:    binding.PoolName,
		handlerName: domain.NormalizeExecutionPoolHandlerName(binding.HandlerName),
	}] = binding
	return nil
}

func (f *fakeBindingRepo) Create(_ context.Context, binding domain.ExecutionPoolBinding) error {
	key := bindingKey{
		poolName:    binding.PoolName,
		handlerName: domain.NormalizeExecutionPoolHandlerName(binding.HandlerName),
	}
	if _, ok := f.bindings[key]; ok {
		return errs.ErrInvalidParameter
	}
	binding.HandlerName = key.handlerName
	f.bindings[key] = binding
	return nil
}

func (f *fakeBindingRepo) CreateBatch(ctx context.Context, bindings []domain.ExecutionPoolBinding) error {
	if f.batchErr != nil {
		return f.batchErr
	}
	for _, binding := range bindings {
		key := bindingKey{poolName: binding.PoolName, handlerName: domain.NormalizeExecutionPoolHandlerName(binding.HandlerName)}
		if _, ok := f.bindings[key]; ok {
			return errs.ErrInvalidParameter
		}
	}
	for _, binding := range bindings {
		if err := f.Create(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeBindingRepo) Unbind(_ context.Context, poolName, handlerName string) error {
	delete(f.bindings, bindingKey{poolName: poolName, handlerName: domain.NormalizeExecutionPoolHandlerName(handlerName)})
	return nil
}

func (f *fakeBindingRepo) SetStatus(
	_ context.Context,
	poolName string,
	handlerName string,
	status domain.ExecutionPoolBindingStatus,
) error {
	key := bindingKey{poolName: poolName, handlerName: domain.NormalizeExecutionPoolHandlerName(handlerName)}
	binding, ok := f.bindings[key]
	if !ok {
		return repository.ErrExecutionPoolBindingNotFound
	}
	binding.Status = status
	f.bindings[key] = binding
	return nil
}

func (f *fakeBindingRepo) Find(
	_ context.Context,
	poolName string,
	handlerName string,
) (domain.ExecutionPoolBinding, error) {
	binding, ok := f.bindings[bindingKey{
		poolName:    poolName,
		handlerName: domain.NormalizeExecutionPoolHandlerName(handlerName),
	}]
	if !ok {
		return domain.ExecutionPoolBinding{}, repository.ErrExecutionPoolBindingNotFound
	}
	return binding, nil
}

func (f *fakeBindingRepo) FindEffective(
	ctx context.Context,
	poolName string,
	handlerName string,
) (domain.ExecutionPoolBinding, error) {
	binding, err := f.Find(ctx, poolName, handlerName)
	if err == nil || domain.NormalizeExecutionPoolHandlerName(handlerName) == "" {
		return binding, err
	}
	return f.Find(ctx, poolName, "")
}

func (f *fakeBindingRepo) CreateDedicatedBatch(
	ctx context.Context,
	_ int64,
	poolName string,
	bindings []domain.ExecutionPoolBinding,
) (bool, error) {
	if f.occupiedPoolNames[poolName] {
		return true, nil
	}
	return false, f.CreateBatch(ctx, bindings)
}

func (f *fakeBindingRepo) List(context.Context, domain.ExecutionPoolBindingStatus) ([]domain.ExecutionPoolBinding, error) {
	return nil, nil
}

func (f *fakeBindingRepo) ListByPool(
	_ context.Context,
	poolName string,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	res := make([]domain.ExecutionPoolBinding, 0)
	for key, binding := range f.bindings {
		if key.poolName != poolName {
			continue
		}
		if status != "" && binding.Status != status {
			continue
		}
		res = append(res, binding)
	}
	return res, nil
}

func (f *fakeBindingRepo) AdminList(
	ctx context.Context,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	return f.List(ctx, status)
}

func (f *fakeBindingRepo) AdminListByPool(
	ctx context.Context,
	poolName string,
	status domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	return f.ListByPool(ctx, poolName, status)
}

func (f *fakeBindingRepo) ListByPoolNames(
	context.Context,
	[]string,
	domain.ExecutionPoolBindingStatus,
) ([]domain.ExecutionPoolBinding, error) {
	return nil, nil
}

func (f *fakeBindingRepo) ListPoolNames(
	_ context.Context,
	cursor string,
	limit int64,
	_ domain.ExecutionPoolBindingStatus,
) ([]string, error) {
	res := make([]string, 0, limit)
	for _, name := range f.poolNames {
		if cursor != "" && name <= cursor {
			continue
		}
		res = append(res, name)
		if int64(len(res)) >= limit {
			break
		}
	}
	return res, nil
}

func (f *fakeBindingRepo) ListAuthorizedPoolNames(
	_ context.Context,
	offset int64,
	limit int64,
	_ string,
	_ domain.ExecutionPoolKind,
	status domain.ExecutionPoolBindingStatus,
	_ domain.ExecutionPoolStatus,
) ([]string, error) {
	names, err := f.ListPoolNames(context.Background(), "", int64(len(f.poolNames)), status)
	if err != nil {
		return nil, err
	}
	if offset > int64(len(names)) {
		return nil, nil
	}
	end := offset + limit
	if end > int64(len(names)) {
		end = int64(len(names))
	}
	return names[int(offset):int(end)], nil
}

func (f *fakeBindingRepo) CountAuthorizedPoolNames(
	_ context.Context,
	_ string,
	_ domain.ExecutionPoolKind,
	status domain.ExecutionPoolBindingStatus,
	_ domain.ExecutionPoolStatus,
) (int64, error) {
	names, err := f.ListPoolNames(context.Background(), "", int64(len(f.poolNames)), status)
	return int64(len(names)), err
}
