package ssh

import (
	"context"
	"io"
	"os/exec"
)

type LocalExec struct {
	ctx    context.Context
	envs   *Envs
	output io.Writer
}

func NewLocalExec(output io.Writer) *LocalExec {
	return &LocalExec{
		output: output,
		envs:   NewEnvs(),
	}
}

func (e *LocalExec) Close() error {
	return nil
}

func (e *LocalExec) WithCtx(ctx context.Context) Command {
	e.ctx = ctx
	return e
}

func (e *LocalExec) WithEnvs(envs *Envs) Command {
	e.envs = envs
	return e
}

func (e *LocalExec) Run(cmd string) error {
	var command *exec.Cmd
	if e.ctx == nil {
		command = exec.Command("bash", "-c", cmd)
	} else {
		command = exec.CommandContext(e.ctx, "bash", "-c", cmd)
	}
	command.Env = e.envs.SliceKV()
	if e.output != nil {
		command.Stderr = e.output
		command.Stdout = e.output
	}
	return command.Run()
}
