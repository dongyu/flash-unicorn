package dto

import "unicorn/internal/domain"

const TokenTypeBearer = "Bearer"

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,max=72"`
}

type LoginResponse struct {
	AccessToken string           `json:"access_token"`
	TokenType   string           `json:"token_type"`
	ExpiresIn   int64            `json:"expires_in"`
	User        AuthUserResponse `json:"user"`
}

type AuthUserResponse struct {
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name"`
	Role        domain.Role       `json:"role"`
	Status      domain.UserStatus `json:"status"`
}

func ToAuthUserResponse(user domain.User) AuthUserResponse {
	return AuthUserResponse{
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
	}
}
