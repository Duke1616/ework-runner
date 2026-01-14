package ioc

import (
	"github.com/meoying/dlock-go"
	dlockRedis "github.com/meoying/dlock-go/redis"
	"github.com/redis/go-redis/v9"
)

func InitDistributedLock(rdb redis.Cmdable) dlock.Client {
	return dlockRedis.NewClient(rdb)
}
