package common

import (
	"gorm.io/gorm"
	"sync"
)

type SpaceWithId struct {
	SpaceId, ID int64
}

var (
	service     *Service
	onceService sync.Once
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	onceService.Do(func() {
		service = &Service{db: db}
	})
	return service
}

// Statistics 发布数据统计
func (srv *Service) Statistics() StatisticsRes {
	return StatisticsRes{}
}

// ServerInfo 系统系统
func (srv *Service) ServerInfo() (*ServerInfo, error) {
	return getServerInfo()
}

// WaitAudit 待审核列表
func (srv *Service) WaitAudit() {

}

// Release 最近发布成功消息
func (srv *Service) Release() {

}
