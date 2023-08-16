package deploy

import (
	"context"
	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"sync"
	"yema.dev/app/model"
)

var Error = errs.Class("Deploy")
var ErrorTaskOver = Error.New("部署任务已完成或未创建")

type Deploy struct {
	mux   *sync.RWMutex
	tasks map[int64]*Task

	db  *gorm.DB
	log *zap.Logger
}

func NewDeploy(db *gorm.DB, log *zap.Logger) *Deploy {
	return &Deploy{
		mux:   &sync.RWMutex{},
		tasks: make(map[int64]*Task),
		db:    db,
		log:   log,
	}
}

// Start 开始部署
func (d *Deploy) Start(taskModel *model.Task) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if _, ok := d.tasks[taskModel.ID]; ok {
		return Error.New("task[%d]已经开始部署", taskModel.ID)
	}
	task, err := NewTask(taskModel, d.db, d.log)
	if err != nil {
		return err
	}
	d.tasks[taskModel.ID] = task
	return d.tasks[taskModel.ID].Start()
}

// Stop 中止部署
func (d *Deploy) Stop(taskId int64) error {
	d.mux.Lock()
	defer d.mux.Unlock()
	if v, ok := d.tasks[taskId]; ok {
		err := v.Stop()
		if err != nil {
			return err
		}
		delete(d.tasks, taskId)
	}
	return nil
}

// Output 部署日志输出
func (d *Deploy) Output(ctx context.Context, taskId int64) (msg <-chan Msg, err error) {
	d.mux.Lock()
	defer d.mux.Unlock()
	if v, ok := d.tasks[taskId]; ok {
		msg = v.Output(ctx)
		return
	}
	return nil, ErrorTaskOver
}
