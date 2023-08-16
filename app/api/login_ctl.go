package api

import (
	"github.com/gin-gonic/gin"
	ctx2 "yema.dev/app/api/ctx"
	"yema.dev/app/internal/errcode"
	"yema.dev/app/internal/response"
	"yema.dev/app/service/common"
	"yema.dev/app/service/login"
)

type LoginCtl struct {
	service *login.Service
}

func (ctl *LoginCtl) Login(ctx *gin.Context) {
	params := login.LoginReq{}
	err := ctx.ShouldBindJSON(&params)
	if err != nil {
		response.Fail(ctx, errcode.ErrInvalidParams.Wrap(err))
		return
	}
	res, err := ctl.service.Login(&params)
	response.Response(ctx, err, res)
}

func (ctl *LoginCtl) Logout(ctx *gin.Context) {
	response.Response(ctx, ctl.service.Logout(ctx2.UserId(ctx)), nil)
}

func (ctl *LoginCtl) RefreshToken(ctx *gin.Context) {
	params := login.RefreshTokenReq{}
	err := ctx.ShouldBindJSON(&params)
	if err != nil {
		response.Fail(ctx, errcode.ErrInvalidParams.Wrap(err))
		return
	}
	res, err := ctl.service.RefreshToken(&params)
	response.Response(ctx, err, res)
}

func (ctl *LoginCtl) UserInfo(ctx *gin.Context) {
	spaceAndId := common.SpaceWithId{
		SpaceId: ctx2.GetSpaceId(ctx),
		ID:      ctx2.UserId(ctx),
	}
	res, err := ctl.service.UserInfo(&spaceAndId)
	response.Response(ctx, err, res)
}
