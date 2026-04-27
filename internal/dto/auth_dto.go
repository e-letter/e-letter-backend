package dto

type RegisterRequest struct {
	Fullname        string `json:"fullname" binding:"required"`
	Email           string `json:"email" binding:"required,email"`
	Password        string `json:"password" binding:"required,min:6"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}
