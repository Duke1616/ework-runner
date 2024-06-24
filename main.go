package main

import (
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/worker"
	"github.com/Duke1616/ecmdb/ioc"
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
	if err = app.WorkerSvc.Start(context.Background(), worker.Worker{
		Name:   cfg.Name,
		Topic:  cfg.Topic,
		Desc:   cfg.Desc,
		Status: 1,
	}); err != nil {
		panic(err)
	}

	err = app.Web.Run(":8001")
	panic(err)
}
