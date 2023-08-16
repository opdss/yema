package global

import (
	"gorm.io/gorm"
	"yema.dev/app/pkg/db"
)

var DB *gorm.DB

func InitDB(conf *db.Config) (err error) {
	DB, err = db.NewGormDB(conf, Log)
	return
}
