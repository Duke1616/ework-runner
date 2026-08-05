package cryptox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCryptoManagerSupportsVersionedAndLegacyCiphertext(t *testing.T) {
	const key = "compatibility-key-longer-than-sixteen-bytes"
	manager := NewCryptoManager("V2").
		Register("V1", MustNewAESCrypto(key)).
		Register("V2", MustNewAESCryptoV2(key)).
		WithLegacyAlgo("V1")

	versioned, err := manager.Encrypt("secret")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(versioned, "ENC:V2:"))
	decrypted, err := manager.Decrypt(versioned)
	require.NoError(t, err)
	require.Equal(t, "secret", decrypted)

	legacy, err := MustNewAESCrypto(key).Encrypt("legacy-secret")
	require.NoError(t, err)
	decrypted, err = manager.Decrypt(legacy)
	require.NoError(t, err)
	require.Equal(t, "legacy-secret", decrypted)

	plainText, err := manager.Decrypt("plain-text")
	require.NoError(t, err)
	require.Equal(t, "plain-text", plainText)
}

func TestCryptoManagerRejectsUnknownVersion(t *testing.T) {
	manager := NewCryptoManager("V2").Register("V2", MustNewAESCryptoV2("key"))
	_, err := manager.Decrypt("ENC:V3:payload")
	require.ErrorContains(t, err, "unsupported encryption version")
}
