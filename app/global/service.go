package global

import (
	s2 "yema.dev/app/service"
	"yema.dev/app/service/deploy"
)

var Service *service

type service struct {
	config *s2.Config
	deploy *deploy.Service
}

func InitService(conf *s2.Config) (err error) {
	Service = &service{
		config: conf,
	}
	return
}

func (s *service) Deploy() *deploy.Service {
	if s.deploy == nil {
		s.deploy = deploy.NewService(DB, Log, Ssh, Repo, &s.config.Deploy)
	}
	return s.deploy
}
