package global

import (
	"go.uber.org/zap"
	"yema.dev/app/pkg/log"
)

var Log *zap.Logger

func InitLog(conf *log.Config) (err error) {
	Log = log.NewLog(conf)
	return
}
