package middleware

import (
	"github.com/gin-gonic/gin"
	"log"
	ctx2 "yema.dev/app/api/ctx"
)

func Auth(ctx *gin.Context) {
	log.Println("middleware auth start")
	jwt, err := ctx2.ValidateBearerToken(ctx)
	if err != nil || jwt.IsRefresh {
		_ = ctx.AbortWithError(401, err)
		return
	}
	ctx.Next()
	log.Println("middleware auth end")
}
