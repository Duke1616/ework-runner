package input

import (
	"encoding/json"
	"testing"

	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/stretchr/testify/require"
)

func TestParameterMerger_Merge(t *testing.T) {
	testCases := []struct {
		name      string
		input     ParameterMergeInput
		want      map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "多层优先级覆盖_由低到高_Runner默认值_任务参数_绑定参数_运行时覆盖",
			input: ParameterMergeInput{
				RunnerDefaults: map[string]json.RawMessage{
					"region":  json.RawMessage(`"default"`),
					"retries": json.RawMessage(`3`),
				},
				TaskParams:       map[string]string{"region": "task", "name": "demo"},
				BindingParams:    map[string]string{"region": "binding"},
				RuntimeOverrides: map[string]string{"region": "runtime"},
			},
			want: map[string]string{
				"region": "runtime", "retries": "3", "name": "demo",
			},
		},
		{
			name: "仅任务参数与运行时覆盖",
			input: ParameterMergeInput{
				TaskParams:       map[string]string{"foo": "bar", "env": "dev"},
				RuntimeOverrides: map[string]string{"env": "test"},
			},
			want: map[string]string{
				"foo": "bar", "env": "test",
			},
		},
		{
			name: "Runner默认参数非法JSON报错",
			input: ParameterMergeInput{
				RunnerDefaults: map[string]json.RawMessage{
					"broken": json.RawMessage(`{invalid-json`),
				},
			},
			wantErr:   true,
			errSubstr: "Runner 默认参数 broken 非法",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (ParameterMerger{}).Merge(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubstr != "" {
					require.ErrorContains(t, err, tc.errSubstr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestVariableMerger_Merge(t *testing.T) {
	testCases := []struct {
		name      string
		layers    []VariableLayer
		want      []variable.Item
		wantErr   bool
		errSubstr string
	}{
		{
			name: "多层覆盖与顺序稳定保持",
			layers: []VariableLayer{
				{Source: VariableSourceRunner, Items: []variable.Item{{Key: "REGION", Value: "cn"}, {Key: "TOKEN", Value: "default", Secret: true}}},
				{Source: VariableSourceTask, Items: []variable.Item{{Key: "TOKEN", Value: "task", Secret: false}, {Key: "DEBUG", Value: "true"}}},
				{Source: VariableSourceBinding, Items: []variable.Item{{Key: "TOKEN", Value: "binding", Secret: true}}},
			},
			want: []variable.Item{
				{Key: "REGION", Value: "cn"},
				{Key: "TOKEN", Value: "binding", Secret: true},
				{Key: "DEBUG", Value: "true"},
			},
		},
		{
			name: "同名覆盖保留底层Secret状态_防止安全降级",
			layers: []VariableLayer{
				{Source: VariableSourceRunner, Items: []variable.Item{
					{Key: "PASSWORD", Value: "initial-secret", Secret: true},
				}},
				{Source: VariableSourceTask, Items: []variable.Item{
					{Key: "PASSWORD", Value: "overridden-value", Secret: false},
				}},
			},
			want: []variable.Item{
				{Key: "PASSWORD", Value: "overridden-value", Secret: true},
			},
		},
		{
			name: "拒绝空Key变量",
			layers: []VariableLayer{
				{Source: VariableSourceTask, Items: []variable.Item{{Value: "invalid"}}},
			},
			wantErr:   true,
			errSubstr: "变量名称不能为空",
		},
		{
			name: "首尾空格被自动Trim",
			layers: []VariableLayer{
				{Source: VariableSourceTask, Items: []variable.Item{{Key: "  TRIMMED_KEY  ", Value: "val"}}},
			},
			want: []variable.Item{
				{Key: "TRIMMED_KEY", Value: "val"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (VariableMerger{}).Merge(tc.layers...)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubstr != "" {
					require.ErrorContains(t, err, tc.errSubstr)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
