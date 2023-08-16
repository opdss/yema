package global

import (
	"yema.dev/app/pkg/ssh"
)

var Ssh *ssh.Ssh

func InitSsh(conf *ssh.Config) (err error) {
	Ssh, err = ssh.NewSSH(conf)
	return
}
