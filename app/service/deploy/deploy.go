package deploy

import (
	"context"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"sync"
	"time"
	"yema.dev/app/model"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
)

var Error = errs.Class("Deploy")
var ErrorTaskFinish = Error.New("部署任务已完成或未创建,未在发布队列中")

type taskRunning struct {
	task   *Task
	cancel func()
}

type deploy struct {
	mux   sync.Mutex
	tasks map[int64]*taskRunning

	db   *gorm.DB
	log  *zap.Logger
	ssh  *ssh.Ssh
	repo *repo.Repos

	MaxDeployNum      int           //最大同时部署任务数量
	MaxReleaseTimeout time.Duration //最大部署超时时间
}

func newDeploy(db *gorm.DB, log *zap.Logger, ssh *ssh.Ssh, repo *repo.Repos, conf *Config) *deploy {
	d := &deploy{
		tasks: make(map[int64]*taskRunning),
		db:    db,
		log:   log,
		ssh:   ssh,
		repo:  repo,
	}
	if conf != nil {
		d.MaxDeployNum = conf.MaxDeploy
		d.MaxReleaseTimeout = conf.MaxReleaseTimeout
	}
	return d
}

// Start 开始部署
func (d *deploy) Start(taskModel *model.Task) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if _, ok := d.tasks[taskModel.ID]; ok {
		return Error.New("该任务[%d]已在部署中", taskModel.ID)
	}
	if len(d.tasks) >= d.MaxDeployNum {
		return Error.New("已经超出部署队列最大数量(%d)，请稍后再试", d.MaxDeployNum)
	}
	task, err := NewTask(taskModel, d.db, d.log, d.ssh, d.repo)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.MaxReleaseTimeout)
	d.tasks[taskModel.ID] = &taskRunning{
		task: task,
		cancel: func() {
			cancel()
		},
	}
	//开始部署
	err = d.tasks[taskModel.ID].task.Start(ctx)
	if err == nil {
		//等待完成处理
		go func() {
			err := d.tasks[taskModel.ID].task.Wait()
			if err != nil {
				d.log.Error("部署任务完成，有错误",
					zap.Int64("taskId", taskModel.ID), zap.Error(err))
			} else {
				d.log.Info("部署任务完成，成功",
					zap.Int64("taskId", taskModel.ID))
			}
		}()
	}
	return err
}

// Stop 中止部署
func (d *deploy) Stop(taskId int64) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if _, ok := d.tasks[taskId]; ok {
		d.tasks[taskId].cancel()
	}
	return ErrorTaskFinish
}

// Output 部署日志输出
func (d *deploy) Output(ctx context.Context, taskId int64) (msg <-chan *ConsoleMsg, err error) {
	d.mux.Lock()
	defer d.mux.Unlock()
	if _, ok := d.tasks[taskId]; ok {
		msg = d.tasks[taskId].task.Output(ctx)
		return
	}
	return nil, ErrorTaskFinish
}
