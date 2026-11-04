package repository

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

var ErrEmailExists = errors.New("email already registered")
var ErrUserNotFound = errors.New("user not found")

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

func (r *Users) FindByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, ErrUserNotFound
	}
	return user, err
}

func (r *Users) FindByID(ctx context.Context, id uint64) (model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, ErrUserNotFound
	}
	return user, err
}
