package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zxxf18/kids-poetry-be/internal/config"
	"github.com/zxxf18/kids-poetry-be/internal/httpapi"
	"github.com/zxxf18/kids-poetry-be/internal/store"
)

func main() {
	configFile := flag.String("f", "etc/backend.example.yaml", "config file")
	flag.Parse()
	data, err := os.ReadFile(*configFile)
	if err != nil {
		panic(err)
	}
	expanded := os.ExpandEnv(string(data))
	var c config.Config
	if err = conf.LoadFromYamlBytes([]byte(expanded), &c); err != nil {
		panic(err)
	}
	s, err := store.Open(c.Database.DSN)
	if err != nil {
		panic(err)
	}
	defer s.Close()
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()
	httpapi.New(s, c.App.DatasetVersion).Register(server)
	logx.Infof("kids poetry API listening on %s:%d", c.Host, c.Port)
	server.Start()
	fmt.Println("stopped")
}
