package middleware

import (
	"errors"
	"github.com/gin-gonic/gin"
	"log"
	ctx2 "yema.dev/app/api/ctx"
	"yema.dev/app/model"
	"yema.dev/app/service/user"
)

func Permission(userService *user.Service, role model.Role) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		log.Println("middleware Permission start")
		userId := ctx2.UserId(ctx)
		spaceId := ctx2.GetSpaceId(ctx)
		if !model.IsSuperUser(userId) {
			if spaceId == 0 {
				_ = ctx.AbortWithError(400, errors.New("未选择空间"))
				return
			}
			member, err := userService.SpaceById(userId, spaceId)
			if err != nil {
				_ = ctx.AbortWithError(400, errors.New("空间选择错误"))
				return
			}
			currRole := member.Role
			if model.Role(currRole).Level() < role.Level() {
				_ = ctx.AbortWithError(401, errors.New("你没有权限访问该空间，请联系相关负责人"))
				return
			}
		}
		//ctx2.SetRole(ctx, currRole)
		ctx.Next()
		log.Println("middleware Permission end")
	}
}
