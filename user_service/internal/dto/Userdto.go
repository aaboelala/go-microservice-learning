package dto

import "time"

// RegisterInput defines the payload for user registration.
type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginInput defines the payload for authenticating a user.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UpdateUserInput defines fields a user can update on their profile.
type UpdateUserInput struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UserResponse defines the safe user payload returned to HTTP clients.
type UserResponse struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoginResponse defines the payload returned upon successful login.
type LoginResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}
