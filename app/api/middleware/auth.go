package middleware

import (
	"github.com/gin-gonic/gin"
	ctx2 "yema.dev/app/api/ctx"
)

func Auth(ctx *gin.Context) {
	jwt, err := ctx2.ValidateBearerToken(ctx)
	if err != nil || jwt.IsRefresh {
		_ = ctx.AbortWithError(401, err)
		return
	}
	ctx.Next()
}
