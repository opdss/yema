package deploy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/wuzfei/go-helper/slices"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"sync"
	"time"
	"yema.dev/app/internal/errcode"
	"yema.dev/app/model"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
	"yema.dev/app/service/common"
	"yema.dev/app/utils"
)

var (
	service     *Service
	onceService sync.Once
)

type Config struct {
	MaxDeploy         int           `help:"最大同时发布数量" default:"10"`
	MaxReleaseTimeout time.Duration `help:"发布超时时间" default:"10m"`
}

type Service struct {
	db     *gorm.DB
	log    *zap.Logger
	deploy *deploy
}

func NewService(db *gorm.DB, log *zap.Logger, ssh *ssh.Ssh, repo *repo.Repos, conf *Config) *Service {
	onceService.Do(func() {
		service = &Service{
			db:     db,
			log:    log,
			deploy: newDeploy(db, log, ssh, repo, conf),
		}
	})
	return service
}

func (srv *Service) List(params *ListReq) (total int64, list []*model.Task, err error) {
	_db := srv.db.Model(&model.Task{}).Where("space_id=?", params.SpaceId)
	err = _db.Count(&total).Error
	if err != nil || total == 0 {
		return
	}
	err = _db.Scopes(params.PageQuery()).
		Preload("User").
		Preload("Project").
		Preload("Environment").
		Order("id desc").
		Find(&list).Error
	return
}

// Create 创建上线单
func (srv *Service) Create(params *CreateReq) error {
	project := &model.Project{SpaceId: params.SpaceId, ID: params.ProjectId}
	err := srv.db.Model(&project).Where(project).Preload("Environment").Preload("Servers").First(&project).Error
	if err != nil {
		return err
	}
	if !project.Status.IsEnable() || !project.Environment.Status.IsEnable() {
		return errors.New("该项目或者该环境暂停上线，请联系相关负责人")
	}
	serverIds := slices.Map(project.Servers, func(item model.Server, k int) int64 {
		return item.ID
	})
	m := &model.Task{
		Name:          params.Name,
		SpaceId:       params.SpaceId,
		UserId:        params.UserId,
		ProjectId:     project.ID,
		EnvironmentId: project.Environment.ID,
		Tag:           params.Tag,
		Branch:        params.Branch,
		CommitId:      params.CommitId,
	}
	m.Status = model.TaskStatusAudit
	if project.IsTaskAudit() {
		m.Status = model.TaskStatusWaiting
	}
	servers := make([]model.Server, 0)
	return srv.db.Transaction(func(tx *gorm.DB) error {
		serverIds = slices.Intersect(serverIds, params.ServerIds)
		if len(serverIds) == 0 {
			return errcode.ErrRequest.Wrap(errors.New("服务器选择错误"))
		}
		err := tx.Where("space_id = ? and id in ?", params.SpaceId, serverIds).Find(&servers).Error
		if err != nil {
			return err
		}
		if len(servers) == 0 {
			return errcode.ErrRequest.Wrap(errors.New("服务器选择错误"))
		}
		m.Servers = servers
		return tx.Create(m).Error
	})
}

// Detail 上线单详情
func (srv *Service) Detail(spaceAndId *common.SpaceWithId) (taskDetail *model.Task, err error) {
	taskDetail = &model.Task{}
	err = srv.db.Where(spaceAndId).
		Preload("Project").
		Preload("Servers").
		First(&taskDetail).
		Error
	return
}

// Delete 删除
func (srv *Service) Delete(spaceId int64) (m *model.Space, err error) {
	err = srv.db.First(&m, spaceId).Error
	return
}

// Audit 审核
func (srv *Service) Audit(params *AuditReq) (err error) {
	var m *model.Task
	err = srv.db.Where("space_id = ? and id = ?", params.SpaceId, params.ID).First(&m).Error
	if err != nil {
		return
	}
	if m.Status != model.TaskStatusWaiting {
		return errors.New("审核失败，该上线单并未处于待审核状态")
	}

	m.AuditUserId = params.AuditUserId
	if params.Audit {
		m.Status = model.TaskStatusAudit
	} else {
		m.Status = model.TaskStatusReject
	}
	m.AuditTime = sql.NullTime{Time: time.Now(), Valid: true}
	return srv.db.Select("status", "audit_user_id", "audit_time").Updates(&m).Error
}

// Release 发布
func (srv *Service) Release(spaceAndId *common.SpaceWithId, userId int64) (err error) {
	//上线单详情
	taskDetail, err := srv.getTask(spaceAndId, "Project", "Environment", "Servers")
	if err != nil {
		return
	}
	return srv.deploy.Start(taskDetail)
}

// StopRelease 停止发布
func (srv *Service) StopRelease(spaceAndId *common.SpaceWithId) (err error) {
	//上线单详情
	taskDetail, err := srv.getTask(spaceAndId, "Project", "Environment", "Servers")
	if err != nil {
		return
	}
	return srv.deploy.Stop(taskDetail.ID)
}

// Rollback 回滚
func (srv *Service) Rollback(spaceId int64) (m *model.Space, err error) {
	err = srv.db.First(&m, spaceId).Error
	return
}

// Console 部署日志控制台输出
func (srv *Service) Console(wsConn *websocket.Conn, spaceAndId *common.SpaceWithId) (err error) {
	defer func() {
		if err != nil {
			srv.log.Error("获取发布日志出错",
				zap.Int64("spaceId", spaceAndId.SpaceId),
				zap.Int64("taskId", spaceAndId.ID),
				zap.Error(err))
		}
	}()
	var taskModel *model.Task
	taskModel, err = srv.getTask(spaceAndId)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg, err := srv.deploy.Output(ctx, taskModel.ID)
	if err != nil {
		if !errors.Is(err, ErrorTaskFinish) {
			return
		}
		msg, err = srv.getReleaseFromDb(ctx, taskModel.ID)
		if err != nil {
			return
		}
	}

	for _msg := range msg {
		select {
		case <-ctx.Done():
			return
		default:
			str, _ := json.Marshal(_msg)
			_err := wsConn.WriteMessage(websocket.TextMessage, str)
			if _err != nil {
				srv.log.Error("ws发送失败", zap.Error(err))
				return
			}
		}
	}
	return
}

// getReleaseFromDb 从数据库读取部署日志
func (srv *Service) getReleaseFromDb(ctx context.Context, taskId int64) (_ <-chan *ConsoleMsg, err error) {
	res := make([]*model.Record, 0)
	if err = srv.db.Where("task_id = ?", taskId).Preload("Server").Order("created_at asc").Find(&res).Error; err != nil {
		return
	}
	msg := make(chan *ConsoleMsg)
	go func() {
		defer close(msg)
		for _, v := range res {
			select {
			case <-ctx.Done():
				return
			default:
				var host string
				if v.Server.ID == 0 {
					host = utils.CurrentHost
				} else {
					host = v.Server.Hostname()
				}
				msg <- &ConsoleMsg{
					ServerId: v.Server.ID,
					Data:     fmt.Sprintf("%s $ %s\r\n%s", host, v.Command, v.Output),
				}
			}
		}
	}()
	return msg, nil
}

func (srv *Service) getTask(spaceAndId *common.SpaceWithId, preloads ...string) (*model.Task, error) {
	//上线单详情
	taskDetail := model.Task{}
	_db := srv.db.Where(spaceAndId)
	for _, pre := range preloads {
		_db = _db.Preload(pre)
	}
	err := _db.First(&taskDetail).Error
	return &taskDetail, err
}
