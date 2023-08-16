package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"go.uber.org/zap"
	ssh2 "golang.org/x/crypto/ssh"
	"gorm.io/gorm"
	"io"
	"os/exec"
	"time"
	"yema.dev/app/global"
	"yema.dev/app/model"
	"yema.dev/app/pkg/ssh"
)

type writer struct {
	buf bytes.Buffer
	w   io.Writer
}

func (w *writer) Write(b []byte) (n int, err error) {
	n, err = w.buf.Write(b)
	if err != nil {
		return 0, err
	}
	return w.w.Write(b)
}

func (w *writer) Bytes() []byte {
	return w.buf.Bytes()
}

type Record struct {
	db      *gorm.DB
	log     *zap.Logger
	model   *model.Record
	server  *model.Server
	envs    *ssh.Envs
	writer  io.Writer
	_writer *writer
}

func NewRecordLocal(db *gorm.DB, log *zap.Logger, taskId, userId int64, cmd string, envs *ssh.Envs, releaseWriter io.Writer) *Record {
	return &Record{
		model: &model.Record{
			UserId:   userId,
			TaskId:   taskId,
			Command:  cmd,
			ServerId: 0,
			Status:   -1,
			Envs:     envs.SliceKV(),
		},
		writer: releaseWriter,

		db:  db,
		log: log,
	}
}

func NewRecordRemote(db *gorm.DB, log *zap.Logger, taskId, userId int64, cmd string, server *model.Server, envs *ssh.Envs, releaseWriter io.Writer) *Record {
	return &Record{
		model: &model.Record{
			UserId:   userId,
			TaskId:   taskId,
			Command:  cmd,
			ServerId: server.ID,
			Status:   -1,
			Envs:     envs.SliceKV(),
		},
		writer: releaseWriter,

		db:  db,
		log: log,
	}
}

func (r *Record) Run(ctx context.Context) (err error) {
	startT := time.Now()
	var command ssh.Command
	var wr *writer
	if r.server == nil {
		r.writer.Write([]byte("wuxin@localhost $\n"))
		wr = &writer{buf: bytes.Buffer{}, w: r.writer}
		command = ssh.NewLocalExec(wr)
	} else {
		r.writer.Write([]byte("wuxin@localhost $\n"))
		wr = &writer{buf: bytes.Buffer{}, w: r.writer}
		command, err = global.Ssh.NewRemoteExec(ssh.ServerConfig{
			Host:     r.server.Host,
			User:     r.server.User,
			Password: "",
			Port:     r.server.Port,
		}, wr)
	}
	if err == nil {
		err = command.WithEnvs(r.envs).WithCtx(ctx).Run(r.model.Command)
	}
	if err != nil {
		if e, ok := err.(*ssh2.ExitError); ok {
			r.model.Status = e.ExitStatus()
		} else if e, ok := err.(*exec.ExitError); ok {
			r.model.Status = e.ExitCode()
		} else {
			r.model.Status = 255
		}
	} else {
		r.model.Status = 0
	}
	r._writer = wr
	r.model.RunTime = time.Since(startT).Milliseconds()
	return r.save()
}

func (r *Record) Save(status int, output *string, runtime int64) error {
	r.model.RunTime = runtime
	r.model.Status = status
	r.model.Output = *output
	return r.save()
}

func (r *Record) Output() string {
	if r._writer == nil {
		return ""
	}
	return string(r._writer.buf.Bytes())
}

func (r *Record) save() error {
	err := r.db.Create(r.model).Error
	if err != nil {
		obj, _ := json.Marshal(r.model)
		r.log.Error("保存执行记录失败", zap.ByteString("record", obj))
	}
	return err
}
