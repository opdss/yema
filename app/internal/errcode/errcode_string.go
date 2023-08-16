// Code generated by "stringer -type ErrCode -linecomment ./app/internal/errcode"; DO NOT EDIT.

package errcode

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Successes-0]
	_ = x[ErrServer-1]
	_ = x[ErrInvalidParams-2]
	_ = x[ErrTimeOut-3]
	_ = x[ErrForbidden-4]
	_ = x[ErrUnauthorized-5]
	_ = x[ErrRequest-6]
	_ = x[ErrInvalidPwd-7]
	_ = x[ErrUserDisabled-8]
	_ = x[ErrCaptcha-9]
	_ = x[ErrNotFound-10]
	_ = x[ErrDataHasExist-11]
}

const _ErrCode_name = "ok服务器处理失败无效参数处理超时无权限访问未授权请求错误用户名或密码错误账号被禁用验证码错误资源未找到或没有权限访问数据已经存在"

var _ErrCode_index = [...]uint8{0, 2, 23, 35, 47, 62, 71, 83, 107, 122, 137, 173, 191}

func (i ErrCode) String() string {
	if i < 0 || i >= ErrCode(len(_ErrCode_index)-1) {
		return "ErrCode(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _ErrCode_name[_ErrCode_index[i]:_ErrCode_index[i+1]]
}
