package api

import (
	"github.com/gin-gonic/gin"
	"yema.dev/app/internal/response"
	"yema.dev/app/service/common"
	"yema.dev/app/version"
)

type CommonCtl struct {
	service *common.Service
}

func (*CommonCtl) Version(ctx *gin.Context) {
	response.Success(ctx, version.Build)
}

func (ctl *CommonCtl) ServiceInfo(ctx *gin.Context) {
	info, err := ctl.service.ServerInfo()
	response.Response(ctx, err, info)
}
