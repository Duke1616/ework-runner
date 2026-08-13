package ioc

import (
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// InitRedis 创建实时事件使用的 Redis 客户端。
// Redis 未配置地址时返回 nil，SSE Hub 会自动降级为进程内广播。
func InitRedis() redis.UniversalClient {
	addr := viper.GetString("redis.addr")
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: viper.GetString("redis.password"),
		DB:       viper.GetInt("redis.db"),
	})
}
