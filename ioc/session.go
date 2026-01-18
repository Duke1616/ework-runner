package ioc

import (
	"time"

	"github.com/ecodeclub/ginx/session"
	"github.com/ecodeclub/ginx/session/cookie"
	"github.com/ecodeclub/ginx/session/header"
	"github.com/ecodeclub/ginx/session/mixin"
	ginRedis "github.com/ecodeclub/ginx/session/redis"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

func InitSession(cmd redis.Cmdable) session.Provider {
	type Config struct {
		SessionEncryptedKey string `mapstructure:"session_encrypted_key"`
		Cookie              struct {
			Domain string `mapstructure:"domain"`
			Name   string `mapstructure:"name"`
		} `mapstructure:"cookie"`
	}
	var cfg Config

	err := viper.UnmarshalKey("session", &cfg)
	if err != nil {
		panic(err)
	}

	if cfg.SessionEncryptedKey == "" {
		panic("session_encrypted_key is required")
	}
	if cfg.SessionEncryptedKey == "" {
		panic("session_encrypted_key is required")
	}
	if cfg.Cookie.Name == "" {
		panic("cookie.name is required")
	}
	if cfg.Cookie.Domain == "" {
		panic("cookie.domain is required")
	}

	const day = time.Hour * 24 * 30
	sp := ginRedis.NewSessionProvider(cmd, cfg.SessionEncryptedKey, day)
	cookieC := &cookie.TokenCarrier{
		MaxAge:   int(day.Seconds()),
		Name:     cfg.Cookie.Name,
		Secure:   true,
		HttpOnly: false,
		Domain:   cfg.Cookie.Domain,
	}
	headerC := header.NewTokenCarrier()
	sp.TokenCarrier = mixin.NewTokenCarrier(headerC, cookieC)
	return sp
}
