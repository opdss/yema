package deploy

import (
	"gorm.io/gorm"
	"testing"
	"yema.dev/app/model"
	"yema.dev/app/pkg/db"
	"yema.dev/app/pkg/log"
)

var testDeployId = 3
var db1 *gorm.DB

func testNewDeploy() (_ *deploy, err error) {
	log1 := log.NewLog(&log.Config{
		File:        "/Users/wuxin/yema.dev.log",
		FileSize:    500,
		FileBackups: 10,
		FileAge:     0,
		Level:       "debug",
		Output:      "any",
		Encoder:     "console",
	})
	db1, err = db.NewGormDB(&db.Config{
		Driver:   "sqlite3",
		Dsn:      "/Users/wuxin/worker/yema/yema_dev.db",
		LogLevel: "",
	}, log1)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func TestNewDeploy(t *testing.T) {
	_, e := testNewDeploy()
	if e != nil {
		t.Fatal(e)
	}
}

func TestDeployStart(t *testing.T) {
	d, e := testNewDeploy()
	if e != nil {
		t.Fatal(e)
	}
	m := model.Task{}
	err := db1.Preload("Servers").Preload("Project").Preload("Environment").Where("id = ?", testDeployId).First(&m).Error
	if err != nil {
		t.Fatal(err)
	}
	err = d.Start(&m)
	if err != nil {
		t.Fatal(err)
	}
}
