package ssh

import "context"

type Command interface {
	WithCtx(ctx context.Context) Command
	WithEnvs(envs *Envs) Command
	Run(cmd string) error
	Close() error
}
