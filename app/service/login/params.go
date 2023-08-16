package login

import "yema.dev/app/model/field"

type LoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=30"`
	Remember bool   `json:"remember" binding:"omitempty"`
}

type LoginRes struct {
	UserId             int64  `json:"user_id"`
	Token              string `json:"token"`
	TokenExpire        int64  `json:"token_expire"`
	RefreshToken       string `json:"refresh_token"`
	RefreshTokenExpire int64  `json:"refresh_token_expire"`
}

type RefreshTokenReq struct {
	RefreshToken string `json:"refresh_token"`
}

type GetUserInfoRes struct {
	UserID         int64        `json:"user_id"`
	Username       string       `json:"username"`
	Email          string       `json:"email"`
	Role           string       `json:"role"`
	Status         field.Status `json:"status"`
	CurrentSpaceId int64        `json:"current_space_id"`
	Spaces         SpaceItems   `json:"spaces"`
}

type SpaceItem struct {
	SpaceName string       `json:"space_name"`
	SpaceId   int64        `json:"space_id"`
	Status    field.Status `json:"status"`
	Role      string       `json:"role"`
}

type SpaceItems []*SpaceItem

func (s SpaceItems) Default(spaceId int64) *SpaceItem {
	if s == nil || len(s) == 0 {
		return nil
	}
	if spaceId != 0 {
		for _, v := range s {
			if v.SpaceId == spaceId {
				return v
			}
		}
	}
	return s[0]
}
