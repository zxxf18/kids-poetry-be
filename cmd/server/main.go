package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zxxf18/kids-poetry-be/internal/audiostore"
	"github.com/zxxf18/kids-poetry-be/internal/config"
	"github.com/zxxf18/kids-poetry-be/internal/httpapi"
	"github.com/zxxf18/kids-poetry-be/internal/store"
)

func main() {
	configFile := flag.String("f", "etc/backend.example.yaml", "config file")
	healthcheck := flag.Bool("healthcheck", false, "check the local API and exit")
	flag.Parse()
	if *healthcheck {
		client := http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://127.0.0.1:8890/api/v1/healthz")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}
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
	audio, err := audiostore.New(c)
	if err != nil {
		panic(err)
	}
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()
	httpapi.New(s, audio, c.App.DatasetVersion).Register(server)
	logx.Infof("kids poetry API listening on %s:%d", c.Host, c.Port)
	server.Start()
	fmt.Println("stopped")
}
