package ssh

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type RemoteExec struct {
	client *client
	envs   *Envs
	ctx    context.Context
	output io.Writer
}

func (e *RemoteExec) Close() error {
	e.client.done()
	return nil
}

func (e *RemoteExec) WithCtx(ctx context.Context) Command {
	e.ctx = ctx
	return e
}
func (e *RemoteExec) WithEnvs(envs *Envs) Command {
	e.envs = envs
	return e
}

func (e *RemoteExec) Run(cmd string) error {
	sess, err := e.client.client.NewSession()
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = sess.Close()
			closed = true
		}
	}()
	e.client.add()
	defer e.client.done()
	if !e.envs.Empty() {
		cmd = fmt.Sprintf("%s && %s", strings.Join(e.envs.SliceKV(), " "), cmd)
	}
	if e.output != nil {
		sess.Stdout = e.output
		sess.Stderr = e.output
	}
	if e.ctx != nil {
		go func() {
			select {
			case <-e.ctx.Done():
				if !closed {
					_ = sess.Close()
					closed = true
				}
				return
			}
		}()
	}
	return sess.Run(cmd)
}
