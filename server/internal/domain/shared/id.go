package shared

// Typed IDs for core aggregates.
// These are simple string newtypes so we can start
// migrating call sites gradually without breaking JSON.

type TenantID string
type UserID string
type ProjectID string
type TaskID string

func NewTenantID(v string) TenantID  { return TenantID(v) }
func NewUserID(v string) UserID      { return UserID(v) }
func NewProjectID(v string) ProjectID { return ProjectID(v) }
func NewTaskID(v string) TaskID      { return TaskID(v) }

