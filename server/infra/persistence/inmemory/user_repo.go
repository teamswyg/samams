package inmemory

import (
	"context"
	"sync"

	"server/internal/domain/shared"
	domainUser "server/internal/domain/user"
	appUser "server/internal/app/user"
)

var _ appUser.UserRepository = (*UserRepository)(nil)

// UserRepository is an in-memory implementation of UserRepository.
type UserRepository struct {
	mu    sync.RWMutex
	store map[domainUser.ID]*domainUser.User
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		store: make(map[domainUser.ID]*domainUser.User),
	}
}

func (r *UserRepository) GetByID(_ context.Context, id domainUser.ID) (*domainUser.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.store[id]
	if !ok {
		return nil, shared.NotFoundError{Resource: "user", ID: string(id)}
	}
	return u, nil
}

func (r *UserRepository) GetByEmail(_ context.Context, email string) (*domainUser.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.store {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, shared.NotFoundError{Resource: "user", ID: email}
}

func (r *UserRepository) FindByGoogleSub(_ context.Context, sub string) (*domainUser.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.store {
		if u.GoogleSub == sub {
			return u, nil
		}
	}
	return nil, shared.NotFoundError{Resource: "user", ID: sub}
}

func (r *UserRepository) Save(_ context.Context, u *domainUser.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[u.ID] = u
	return nil
}
