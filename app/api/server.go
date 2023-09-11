package api

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/wuzfei/cfgstruct/cfgstruct"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"net"
	"net/http"
	"yema.dev/app/internal/validate"
	"yema.dev/app/pkg/jwt"
	"yema.dev/app/pkg/repo"
	"yema.dev/app/pkg/ssh"
)

type Config struct {
	BaseUrl string `help:"访问地址" devDefault:"http://localhost:8989/" default:"http://localhost:9000"`
	Address string `help:"监听地址" devDefault:"0.0.0.0:8989" default:"0.0.0.0:9000"`
}

type Server struct {
	rootFs   *embed.FS
	assetsFs *embed.FS
	server   http.Server
	config   *Config

	Log  *zap.Logger
	Jwt  *jwt.Jwt
	DB   *gorm.DB
	Repo *repo.Repos
	Ssh  *ssh.Ssh
}

func NewServer(conf *Config, rootfs *embed.FS, assets *embed.FS) *Server {
	server := &Server{
		rootFs:   rootfs,
		assetsFs: assets,
		config:   conf,
	}
	return server
}

func (s *Server) Run(ctx context.Context) error {
	if cfgstruct.DefaultsType() == cfgstruct.DefaultsRelease {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.Default()
	RegisterRoutes(engine, s)
	// 注册自定义验证标签
	if err := validate.RegisterValidation(); err != nil {
		return err
	}
	s.server.Handler = engine

	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	var group errgroup.Group
	group.Go(func() error {
		<-ctx.Done()
		return s.server.Shutdown(context.Background())
	})
	group.Go(func() error {
		defer cancel()
		_err := s.server.Serve(listener)
		if errors.Is(_err, http.ErrServerClosed) {
			_err = nil
		}
		return _err
	})
	fmt.Println("访问地址：", s.config.BaseUrl)
	return group.Wait()
}
