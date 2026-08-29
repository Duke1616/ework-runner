package security

import (
	"testing"

	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/stretchr/testify/require"
)

func TestVariableProtectorSupportsUnversionedHistoricalCiphertext(t *testing.T) {
	const key = "compatibility-key-longer-than-sixteen-bytes"
	historical := cryptox.MustNewAESCrypto(key)
	legacyValue, err := historical.Encrypt("legacy-secret")
	require.NoError(t, err)

	manager := cryptox.NewCryptoManager("V2").
		Register("V1", historical).
		Register("V2", cryptox.MustNewAESCryptoV2(key)).
		WithLegacyAlgo("V1")
	protector := NewVariableProtector(cryptox.NewValueProtector(manager))

	items, err := protector.DecryptVariables([]variable.Item{{Key: "wechat_secret", Value: legacyValue, Secret: true}})
	require.NoError(t, err)
	require.Equal(t, "legacy-secret", items[0].Value)
}

func TestVariableProtectorRejectsPlaintextSecret(t *testing.T) {
	manager := cryptox.NewCryptoManager("V2").Register("V2", cryptox.MustNewAESCryptoV2("key"))
	protector := NewVariableProtector(cryptox.NewValueProtector(manager))

	_, err := protector.DecryptVariables([]variable.Item{{Key: "wechat_secret", Value: "plain-text", Secret: true}})
	require.ErrorContains(t, err, "不是合法密文")
}
