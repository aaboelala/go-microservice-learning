package domain

import "time"

// User represents the database model for users.
type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Email        string    `json:"email" gorm:"uniqueIndex;not null;type:varchar(255)"`
	PasswordHash string    `json:"-" gorm:"not null"`
	Name         string    `json:"name" gorm:"not null;type:varchar(100)"`
	Role         string    `json:"role" gorm:"not null;type:varchar(20);default:user"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
