package user

import (
	"time"

	"server/internal/domain/shared"
)

type ID string

type Plan int32

const (
	PlanUnknown Plan = iota
	PlanFree
	PlanPro
	PlanEnterprise
)

type User struct {
	ID            ID
	TenantID      shared.TenantID
	Email         string
	DisplayName   string
	Plan          Plan
	GoogleSub     string
	FirebaseUID   string
	AIAPITokenKey string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Active        bool
}

func NewUser(clock shared.Clock, id ID, tenantID shared.TenantID, email, displayName string, plan Plan) (*User, error) {
	if tenantID == "" {
		return nil, shared.ValidationError{Msg: "tenant_id is required"}
	}
	if email == "" {
		return nil, shared.ValidationError{Msg: "email is required"}
	}
	now := clock.Now()
	return &User{
		ID:          id,
		TenantID:    tenantID,
		Email:       email,
		DisplayName: displayName,
		Plan:        plan,
		CreatedAt:   now,
		UpdatedAt:   now,
		Active:      true,
	}, nil
}

func (u *User) Activate(clock shared.Clock) error {
	if u == nil {
		return shared.ValidationError{Msg: "user is nil"}
	}
	if u.Active {
		return nil
	}
	u.Active = true
	u.UpdatedAt = clock.Now()
	return nil
}

func (u *User) Deactivate(clock shared.Clock) error {
	if u == nil {
		return shared.ValidationError{Msg: "user is nil"}
	}
	if !u.Active {
		return nil
	}
	u.Active = false
	u.UpdatedAt = clock.Now()
	return nil
}

