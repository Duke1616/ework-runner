package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/ioc"
	"github.com/Duke1616/ework-runner/pkg/registry"
	"github.com/Duke1616/ework-runner/pkg/registry/etcd"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	initViper()

	app, err := ioc.InitApp()
	if err != nil {
		panic(err)
	}

	var cfg registry.Instance
	if err = viper.UnmarshalKey("worker", &cfg); err != nil {
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

	err = app.Web.Run(":8001")
	panic(err)
}

func initViper() {
	file := pflag.String("config",
		"config/prod.yaml", "配置文件路径")
	pflag.Parse()
	// 直接指定文件路径
	viper.SetConfigFile(*file)
	viper.WatchConfig()
	err := viper.ReadInConfig()

	if err != nil {
		panic(err)
	}
}
