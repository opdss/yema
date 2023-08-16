package user

import (
	"yema.dev/app/model/field"
	"yema.dev/app/pkg/db"
)

type CreateReq struct {
	Username string       `json:"username" binding:"required,max=30"`
	Email    string       `json:"email" binding:"required,email"`
	Password string       `json:"password" binding:"required,min=6,max=50"`
	Status   field.Status `json:"status" binding:"omitempty"`
}

type ListReq struct {
	Keyword string `json:"keyword" query:"keyword" form:"keyword" binding:"omitempty"`
	db.Paginator
}

type MemberListReq struct {
	SpaceId int64
	db.Paginator
}

type MemberListItem struct {
}

type UpdateReq struct {
	ID       int64        `json:"id"  binding:"required,min=1"`
	Username string       `json:"username" binding:"required,max=30"`
	Email    string       `json:"email" binding:"required,email"`
	Password string       `json:"password" binding:"omitempty,min=6,max=50"`
	Status   field.Status `json:"status" binding:"omitempty"`
}
