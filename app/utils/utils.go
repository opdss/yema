package utils

import (
	"github.com/wuzfei/cfgstruct/cfgstruct"
)

func IsDev() bool {
	env := cfgstruct.DefaultsType()
	return env == "" || env == "dev"
}
