package repository

import (
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/pkg/security"
	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/stretchr/testify/require"
)

func TestTaskEntityProtectsStructuredVariables(t *testing.T) {
	repo := &taskRepository{protector: security.NewVariableProtector(cryptox.NewValueProtector(taskPayloadCipher{}))}
	entity, err := repo.toEntity(domain.Task{GrpcConfig: &domain.GrpcConfig{
		Variables: []variable.Item{{Key: "token", Value: "top-secret", Secret: true}},
	}})
	require.NoError(t, err)
	require.Equal(t, "ENC:test", entity.GrpcConfig.Val.Variables[0].Value)
}

type taskPayloadCipher struct{}

func (taskPayloadCipher) Encrypt(string) (string, error) { return "ENC:test", nil }
func (taskPayloadCipher) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "ENC:test"), nil
}
