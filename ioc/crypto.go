package ioc

import (
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/spf13/viper"
)

// InitCrypto 初始化执行单元变量加解密组件。
func InitCrypto() cryptox.Crypto {
	type Config struct {
		Version        string   `mapstructure:"version"`
		Key            string   `mapstructure:"key"`
		LegacyVersions []string `mapstructure:"legacy_versions"`
	}

	var cfg Config
	if err := viper.UnmarshalKey("encryption", &cfg); err != nil {
		panic(err)
	}
	if cfg.Version == "" {
		panic("missing required config: encryption.version")
	}
	if cfg.Key == "" {
		panic("missing required config: encryption.key")
	}

	legacyVersions := cfg.LegacyVersions
	if len(legacyVersions) == 0 {
		legacyVersions = []string{cfg.Version}
	}
	return cryptox.NewCryptoManager("V2").
		Register("V2", cryptox.MustNewAESCryptoV2(cfg.Key)).
		Register(cfg.Version, cryptox.MustNewAESCrypto(cfg.Key)).
		WithLegacyAlgos(legacyVersions...)
}
