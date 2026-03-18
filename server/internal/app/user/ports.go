package user

import (
	"context"

	domainUser "server/internal/domain/user"
)

type UserRepository interface {
	GetByID(ctx context.Context, id domainUser.ID) (*domainUser.User, error)
	GetByEmail(ctx context.Context, email string) (*domainUser.User, error)
	FindByGoogleSub(ctx context.Context, sub string) (*domainUser.User, error)
	Save(ctx context.Context, u *domainUser.User) error
}

