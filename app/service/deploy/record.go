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
	"sync"
	"time"
	"yema.dev/app/model"
	"yema.dev/app/pkg/ssh"
)

type cmdOutput struct {
	mux sync.RWMutex
	buf bytes.Buffer
	w   io.Writer
}

func (w *cmdOutput) Write(b []byte) (n int, err error) {
	w.mux.Lock()
	defer w.mux.Unlock()
	n, err = w.buf.Write(b)
	if err != nil {
		return n, err
	}
	return w.w.Write(b)
}

func (w *cmdOutput) Bytes() []byte {
	w.mux.RLock()
	defer w.mux.RUnlock()
	return w.buf.Bytes()
}

type Record struct {
	db  *gorm.DB
	log *zap.Logger
	ssh *ssh.Ssh

	model  *model.Record
	server *model.Server
	envs   *ssh.Envs
	output io.Writer //此次执行日志

	startTime time.Time
}

func NewRecordLocal(db *gorm.DB, log *zap.Logger, ssh *ssh.Ssh, taskId, userId int64, cmd string, envs *ssh.Envs, releaseOutput io.Writer) *Record {
	return &Record{
		model: &model.Record{
			UserId:   userId,
			TaskId:   taskId,
			Command:  cmd,
			ServerId: 0,
			Status:   -1,
			Envs:     envs.SliceKV(),
		},
		output: &cmdOutput{buf: bytes.Buffer{}, w: releaseOutput},

		envs: envs,

		db:  db,
		log: log,
		ssh: ssh,
	}
}

func NewRecordRemote(db *gorm.DB, log *zap.Logger, ssh *ssh.Ssh, taskId, userId int64, cmd string, server *model.Server, envs *ssh.Envs, releaseOutput io.Writer) *Record {
	return &Record{
		model: &model.Record{
			UserId:   userId,
			TaskId:   taskId,
			Command:  cmd,
			ServerId: server.ID,
			Status:   -1,
			Envs:     envs.SliceKV(),
		},

		output: &cmdOutput{buf: bytes.Buffer{}, w: releaseOutput},

		server: server,
		envs:   envs,

		db:  db,
		log: log,
		ssh: ssh,
	}
}

func (r *Record) Run(ctx context.Context) (err error) {
	startT := time.Now()
	var command ssh.Command
	if r.server == nil {
		r.log.Info("本地执行命令", zap.String("cmd", r.model.Command))
		command = ssh.NewLocalExec(r.output)
	} else {
		r.log.Info("服务器执行命令", zap.String("cmd", r.model.Command), zap.Int64("server", r.model.ServerId))
		command, err = r.ssh.NewRemoteExec(ssh.ServerConfig{
			Host: r.server.Host,
			User: r.server.User,
			Port: r.server.Port,
		}, r.output)
	}
	if err == nil {
		defer func() {
			_ = command.Close()
		}()
		err = command.WithEnvs(r.envs).RunCtx(ctx, r.model.Command)
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
	r.model.RunTime = time.Now().Sub(startT).Milliseconds()
	return r.save()
}

func (r *Record) SetSaveTime() {
	r.startTime = time.Now()
}

func (r *Record) Save(status int, output string) error {
	if r.startTime.IsZero() {
		r.model.RunTime = 0
	} else {
		r.model.RunTime = time.Since(r.startTime).Milliseconds()
	}
	r.model.Status = status
	r.model.Output = output + "\r\n"
	r.output.Write([]byte(r.model.Output))
	return r.save()
}

func (r *Record) Output() string {
	return string(r.output.(*cmdOutput).Bytes())
}

func (r *Record) save() error {
	err := r.db.Create(r.model).Error
	if err != nil {
		obj, _ := json.Marshal(r.model)
		r.log.Error("保存执行记录失败", zap.ByteString("record", obj))
	}
	return err
}
