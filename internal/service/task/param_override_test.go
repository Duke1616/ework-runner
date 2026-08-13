package task

import (
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/stretchr/testify/require"
)

func TestValidateAndSerializeOverrides(t *testing.T) {
	rules := []domain.TaskParamOverrideRule{{
		ParamKey: "limit",
		AllowedModes: []domain.TaskParamInputMode{
			domain.TaskParamInputModeManual,
			domain.TaskParamInputModeSelect,
		},
		DefaultMode: domain.TaskParamInputModeSelect,
		SelectConfig: &domain.TaskParamSelectConfig{
			Multiple: true,
			Options: []domain.TaskParamOption{
				{Label: "Host 01", Value: "host01"},
				{Label: "Host 02", Value: "host02"},
			},
		},
	}}

	t.Run("manual host pattern", func(t *testing.T) {
		got, err := validateAndSerializeOverrides(rules, map[string]domain.RunParamOverride{
			"limit": {Mode: domain.TaskParamInputModeManual, Value: "web:&prod:!disabled"},
		})
		require.NoError(t, err)
		require.Equal(t, "web:&prod:!disabled", got["limit"])
	})

	t.Run("multiple selection preserves order", func(t *testing.T) {
		got, err := validateAndSerializeOverrides(rules, map[string]domain.RunParamOverride{
			"limit": {Mode: domain.TaskParamInputModeSelect, Values: []string{"host02", "host01"}},
		})
		require.NoError(t, err)
		require.Equal(t, "host02,host01", got["limit"])
	})

	for _, tc := range []struct {
		name     string
		override domain.RunParamOverride
	}{
		{name: "unknown option", override: domain.RunParamOverride{Mode: domain.TaskParamInputModeSelect, Values: []string{"host03"}}},
		{name: "duplicate option", override: domain.RunParamOverride{Mode: domain.TaskParamInputModeSelect, Values: []string{"host01", "host01"}}},
		{name: "unsupported mode", override: domain.RunParamOverride{Mode: "OTHER", Value: "host01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateAndSerializeOverrides(rules, map[string]domain.RunParamOverride{"limit": tc.override})
			require.Error(t, err)
			require.True(t, errors.Is(err, errs.ErrInvalidParameter))
		})
	}
}

func TestValidateOverrideRule(t *testing.T) {
	t.Run("select mode must be default", func(t *testing.T) {
		err := validateOverrideRule(domain.TaskParamOverrideRule{
			ParamKey: "limit",
			AllowedModes: []domain.TaskParamInputMode{
				domain.TaskParamInputModeManual,
				domain.TaskParamInputModeSelect,
			},
			DefaultMode: domain.TaskParamInputModeManual,
			SelectConfig: &domain.TaskParamSelectConfig{
				Options: []domain.TaskParamOption{{Label: "all", Value: "all"}},
			},
		})
		require.ErrorIs(t, err, errs.ErrInvalidParameter)
	})

	t.Run("select option cannot contain comma", func(t *testing.T) {
		err := validateOverrideRule(domain.TaskParamOverrideRule{
			ParamKey: "limit", AllowedModes: []domain.TaskParamInputMode{domain.TaskParamInputModeSelect},
			DefaultMode: domain.TaskParamInputModeSelect,
			SelectConfig: &domain.TaskParamSelectConfig{
				Options: []domain.TaskParamOption{{Label: "two hosts", Value: "host01,host02"}},
			},
		})
		require.ErrorIs(t, err, errs.ErrInvalidParameter)
	})

	t.Run("single select requires exactly one value", func(t *testing.T) {
		_, err := validateSelectedValues("tags", domain.TaskParamSelectConfig{
			Options: []domain.TaskParamOption{
				{Label: "Start", Value: "start"}, {Label: "Stop", Value: "stop"},
			},
		}, []string{"start", "stop"})
		require.ErrorIs(t, err, errs.ErrInvalidParameter)
	})
}
