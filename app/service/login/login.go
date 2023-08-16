package login

import (
	"errors"
	"github.com/wuzfei/go-helper/slices"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"sync"
	"yema.dev/app/internal/errcode"
	"yema.dev/app/model"
	"yema.dev/app/pkg/jwt"
	"yema.dev/app/service/common"
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

// Login 登陆
func (srv *Service) Login(params *LoginReq) (*LoginRes, error) {
	m := model.User{}
	err := srv.db.Where("email = ?", params.Email).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ErrInvalidPwd
		}
		return nil, err
	}
	if m.Status.IsDisable() {
		return nil, errcode.ErrUserDisabled
	}
	if bcrypt.CompareHashAndPassword(m.Password, []byte(params.Password)) != nil {
		return nil, errcode.ErrInvalidPwd
	}
	//生成token
	res := LoginRes{}
	res.Token, res.TokenExpire, err = srv.jwt.CreateToken(jwt.TokenPayload{
		UserId:   m.ID,
		Email:    m.Email,
		Username: m.Username,
	})
	if err != nil {
		return nil, err
	}
	res.UserId = m.ID
	//记住登陆
	if params.Remember {
		res.RefreshToken, res.RefreshTokenExpire, err = srv.jwt.CreateRefreshToken(jwt.TokenPayload{
			UserId:    m.ID,
			Email:     m.Email,
			Username:  m.Username,
			IsRefresh: true,
		})
		if err != nil {
			return nil, err
		}
		m.RememberToken = res.RefreshToken
		if err = srv.db.Select("remember_token").Updates(&m).Error; err != nil {
			return nil, err
		}
	}
	return &res, nil
}

// RefreshToken 刷新token
func (srv *Service) RefreshToken(params *RefreshTokenReq) (res *LoginRes, err error) {
	jwtClaims, err := srv.jwt.ValidateToken(params.RefreshToken)
	if err != nil {
		return
	}
	m := model.User{}
	err = srv.db.First(&m, jwtClaims.UserId).Error
	if err != nil {
		return
	}
	if m.Status.IsDisable() {
		return nil, errcode.ErrUserDisabled
	}
	if m.RememberToken != params.RefreshToken {
		return nil, errcode.ErrInvalidParams.New("refresh token 错误")
	}

	res = &LoginRes{}
	res.Token, res.TokenExpire, err = srv.jwt.CreateToken(jwt.TokenPayload{
		UserId:   m.ID,
		Email:    m.Email,
		Username: m.Username,
	})
	if err != nil {
		return
	}
	res.UserId = m.ID
	res.RefreshToken, res.RefreshTokenExpire, err = srv.jwt.CreateRefreshToken(jwt.TokenPayload{
		UserId:    m.ID,
		Email:     m.Email,
		Username:  m.Username,
		IsRefresh: true,
	})
	if err != nil {
		return nil, err
	}
	m.RememberToken = res.RefreshToken
	if err = srv.db.Select("remember_token").Updates(&m).Error; err != nil {
		return nil, err
	}
	return
}

// Logout 退出
func (srv *Service) Logout(userId int64) (err error) {
	m := model.User{}
	err = srv.db.First(&m, userId).Error
	if err != nil {
		return
	}
	m.RememberToken = ""
	return srv.db.Select("remember_token").Updates(&m).Error
}

// UserInfo 获取用户信息
func (srv *Service) UserInfo(params *common.SpaceWithId) (userInfo *GetUserInfoRes, err error) {
	m := model.User{}
	err = srv.db.First(&m, params.ID).Error
	if err != nil {
		return
	}
	role := ""
	if m.IsSuperUser() {
		role = string(model.RoleSuper)
	}
	currentSpaceId := params.SpaceId
	//获取所属空间
	spaceItems, err := srv.spacesItems(m.ID)
	if err != nil {
		return
	}
	//根据当前空间，获取当前空间id和角色
	currSpaceItem := spaceItems.Default(currentSpaceId)
	if currSpaceItem != nil {
		currentSpaceId = currSpaceItem.SpaceId
		if role == "" {
			role = currSpaceItem.Role
		}
	} else {
		currentSpaceId = 0
	}

	userInfo = &GetUserInfoRes{
		UserID:         m.ID,
		Email:          m.Email,
		Username:       m.Username,
		Role:           role,
		Status:         m.Status,
		CurrentSpaceId: currentSpaceId,
		Spaces:         spaceItems,
	}
	return
}

// spacesItems 获取用户所属的所有空间
func (srv *Service) spacesItems(userId int64) (SpaceItems, error) {
	spaceItems := make(SpaceItems, 0)
	if !model.IsSuperUser(userId) {
		res, err := srv.spaces(userId)
		if err != nil {
			return spaceItems, err
		}
		spaceItems = slices.Map(res, func(item *model.Member, k int) *SpaceItem {
			return &SpaceItem{
				SpaceId:   item.Space.ID,
				SpaceName: item.Space.Name,
				Status:    item.Space.Status,
				Role:      item.Role,
			}
		})
	} else {
		//超级管理员的处理
		var res []*model.Space
		err := srv.db.Find(&res).Error
		if err != nil {
			return spaceItems, err
		}
		spaceItems = slices.Map(res, func(item *model.Space, k int) *SpaceItem {
			return &SpaceItem{
				SpaceId:   item.ID,
				SpaceName: item.Name,
				Status:    item.Status,
				Role:      string(model.RoleSuper),
			}
		})
	}
	return spaceItems, nil
}

// Spaces 获取一个用户所有空间信息
func (srv *Service) spaces(userId int64) (res []*model.Member, err error) {
	err = srv.db.Where("user_id = ?", userId).Preload("Space").Find(&res).Error
	if err != nil {
		return
	}
	res = slices.FilterFunc(res, func(item *model.Member) bool {
		return item.Space.ID != 0
	})
	return
}
