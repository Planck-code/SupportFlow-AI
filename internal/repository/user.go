package repository

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

var ErrEmailExists = errors.New("email already registered")

type Users struct{ db *gorm.DB }

func NewUsers(db *gorm.DB) *Users { return &Users{db: db} }

func (r *Users) Create(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	var duplicate *mysql.MySQLError
	if errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &duplicate) && duplicate.Number == 1062) {
		return ErrEmailExists
	}
	return err
}
