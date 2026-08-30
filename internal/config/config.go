package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	Database struct {
		DSN string
	}
	App struct {
		DatasetVersion string
	}
}
