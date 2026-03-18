package localstore

import (
	"context"
	"fmt"

	"server/internal/domain/shared"
	domainUser "server/internal/domain/user"
	appUser "server/internal/app/user"
)

var _ appUser.UserRepository = (*UserRepository)(nil)

// UserRepository stores users as JSON files.
// Key: users/{id}.json
type UserRepository struct {
	store *Store
}

func NewUserRepository(store *Store) *UserRepository {
	return &UserRepository{store: store}
}

func (r *UserRepository) key(id domainUser.ID) string {
	return fmt.Sprintf("users/%s.json", id)
}

func (r *UserRepository) GetByID(_ context.Context, id domainUser.ID) (*domainUser.User, error) {
	var u domainUser.User
	if err := r.store.Get(r.key(id), &u); err != nil {
		if err == ErrNotFound {
			return nil, shared.NotFoundError{Resource: "user", ID: string(id)}
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByEmail(_ context.Context, email string) (*domainUser.User, error) {
	keys, err := r.store.List("users")
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		var u domainUser.User
		if err := r.store.Get(k, &u); err != nil {
			continue
		}
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, shared.NotFoundError{Resource: "user", ID: email}
}

func (r *UserRepository) FindByGoogleSub(_ context.Context, sub string) (*domainUser.User, error) {
	keys, err := r.store.List("users")
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		var u domainUser.User
		if err := r.store.Get(k, &u); err != nil {
			continue
		}
		if u.GoogleSub == sub {
			return &u, nil
		}
	}
	return nil, shared.NotFoundError{Resource: "user", ID: sub}
}

func (r *UserRepository) Save(_ context.Context, u *domainUser.User) error {
	return r.store.Put(r.key(u.ID), u)
}
