package user

import (
	"errors"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"sync"
	"yema.dev/app/internal/errcode"
	"yema.dev/app/model"
	"yema.dev/app/pkg/jwt"
)

var (
	service     *Service
	onceService sync.Once
)

type Service struct {
	log *zap.Logger
	db  *gorm.DB
	jwt *jwt.Jwt
}

func NewService(log *zap.Logger, db *gorm.DB, jwt *jwt.Jwt) *Service {
	onceService.Do(func() {
		service = &Service{
			log: log,
			db:  db,
			jwt: jwt,
		}
	})
	return service
}

// Create 创建新用户
func (srv *Service) Create(params *CreateReq) (err error) {
	m := model.User{}
	var exists int64
	err = srv.db.Model(&m).Where("email = ?", params.Email).Count(&exists).Error
	if err != nil {
		return
	}
	if exists != 0 {
		return errcode.ErrInvalidParams.Wrap(errors.New("该用户email已存在"))
	}
	m.Username = params.Username
	m.Email = params.Email
	m.Status = params.Status

	_pwd, err := bcrypt.GenerateFromPassword([]byte(params.Password), bcrypt.DefaultCost)
	if err != nil {
		return
	}
	m.Password = _pwd
	return srv.db.Create(&m).Error
}

// Update 更新用户
func (srv *Service) Update(params *UpdateReq) (err error) {
	m := model.User{}
	err = srv.db.First(&m, params.ID).Error
	if err != nil {
		return
	}
	if m.ID == 0 {
		return errors.New("用户不存在")
	}
	m.ID = params.ID
	m.Username = params.Username
	m.Email = params.Email
	m.Status = params.Status
	if params.Password != "" {
		m.Password, err = bcrypt.GenerateFromPassword([]byte(params.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
	}
	return srv.db.UpdateColumns(&m).Error
}

// Delete 删除用户
func (srv *Service) Delete(id int64) (err error) {
	if model.IsSuperUser(id) {
		return errors.New("超级管理员不允许删除")
	}
	return srv.db.Delete(&model.User{}, id).Error
}

// List 获取列表
func (srv *Service) List(params *ListReq) (total int64, res []*model.User, err error) {
	db := srv.db.Model(&model.User{})
	if params.Keyword != "" {
		_k := "%" + params.Keyword + "%"
		db.Where("username like ? or email like ?", _k, _k)
	}
	err = db.Count(&total).Error
	if err != nil || total == 0 {
		return
	}
	err = db.Scopes(params.PageQuery()).Find(&res).Error
	return
}

func (srv *Service) Members(spaceId int64, params *ListReq) (total int64, res []any, err error) {
	var result []struct {
	}
	_db := srv.db.Model(&model.Member{}).Where(model.Member{SpaceId: spaceId})
	err = _db.Count(&total).Error
	if err != nil || total == 0 {
		return
	}
	err = _db.Scopes(params.PageQuery()).
		Joins("User").
		Scan(&result).
		Error
	return
}

// SpaceById 获取一个用户所有空间信息
func (srv *Service) SpaceById(userId int64, spaceId int64) (res *model.Member, err error) {
	err = srv.db.Where(model.Member{SpaceId: spaceId, UserId: userId}).First(&res).Error
	return
}

func (srv *Service) Detail(userId int64) (m *model.User, err error) {
	err = srv.db.First(&m, userId).Error
	return
}
