package main

import (
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/ioc"
	"github.com/Duke1616/ecmdb/pkg/registry"
	"github.com/Duke1616/ecmdb/pkg/registry/etcd"
	"time"
)

func main() {
	app, err := ioc.InitApp()
	if err != nil {
		panic(err)
	}

	type Config struct {
		Name  string `json:"name"`
		Topic string `json:"topic"`
		Desc  string `json:"desc"`
	}
	var cfg Config
	if err = app.Viper.UnmarshalKey("worker", &cfg); err != nil {
		panic(fmt.Errorf("unable to decode into struct: %v", err))
	}

	r, err := etcd.NewRegistry(app.EtcdClient)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	// 要确保端口启动之后才能注册
	err = r.Register(ctx, registry.Instance{
		Name:  cfg.Name,
		Desc:  cfg.Desc,
		Topic: cfg.Topic,
	})
	cancel()

	if err != nil {
		panic("注册失败")
	}

	//if err = app.WorkerSvc.Start(context.Background(), worker.Worker{
	//	Name:   cfg.Name,
	//	Topic:  cfg.Topic,
	//	Desc:   cfg.Desc,
	//	Status: 1,
	//}); err != nil {
	//	panic(err)
	//}

	err = app.Web.Run(":8001")
	panic(err)
}
