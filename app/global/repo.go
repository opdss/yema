package global

import "yema.dev/app/pkg/repo"

var Repo *repo.Repos

func InitRepo(conf *repo.Config) (err error) {
	Repo, err = repo.NewRepos(conf)
	return
}
