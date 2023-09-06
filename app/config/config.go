package config

import (
	"github.com/zeebo/errs"
	"yema.dev/app/api"
	"yema.dev/app/global"
	"yema.dev/app/pkg/db"
	"yema.dev/app/pkg/jwt"
	"yema.dev/app/pkg/log"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
	"yema.dev/app/service"
)

var Cfg *Config

type Config struct {
	Api     api.Config
	Db      db.Config
	Repo    repo.Config
	JWT     jwt.Config
	Log     log.Config
	Ssh     ssh.Config
	Service service.Config
}

func (conf Config) Init() {
	Cfg = &conf
	errs2 := errs.Group{}
	errs2.Add(
		global.InitLog(&conf.Log),
		global.InitDB(&conf.Db),
		global.InitJwt(&conf.JWT),
		global.InitRepo(&conf.Repo),
		global.InitSsh(&conf.Ssh),
		global.InitService(&conf.Service),
	)
	if errs2.Err() != nil {
		panic(errs2.Err())
	}
}
