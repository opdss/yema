package global

import "yema.dev/app/pkg/jwt"

var Jwt *jwt.Jwt

func InitJwt(conf *jwt.Config) (err error) {
	Jwt, err = jwt.NewJWT(conf)
	return
}
