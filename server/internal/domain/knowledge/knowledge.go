package knowledge

import (
	"time"

	"server/internal/domain/shared"
)

type ID string

type Category int32

const (
	CategoryUnknown Category = iota
	CategoryCodingConvention
	CategoryArchitecturalDecision
	CategoryDomainRule
	CategoryErrorPattern
	CategoryPerformanceInsight
	CategoryWorkflowPattern
	CategoryTeamPreference
)

func (c Category) String() string {
	switch c {
	case CategoryCodingConvention:
		return "coding_convention"
	case CategoryArchitecturalDecision:
		return "architectural_decision"
	case CategoryDomainRule:
		return "domain_rule"
	case CategoryErrorPattern:
		return "error_pattern"
	case CategoryPerformanceInsight:
		return "performance_insight"
	case CategoryWorkflowPattern:
		return "workflow_pattern"
	case CategoryTeamPreference:
		return "team_preference"
	default:
		return "unknown"
	}
}

func ParseCategory(s string) Category {
	switch s {
	case "coding_convention":
		return CategoryCodingConvention
	case "architectural_decision":
		return CategoryArchitecturalDecision
	case "domain_rule":
		return CategoryDomainRule
	case "error_pattern":
		return CategoryErrorPattern
	case "performance_insight":
		return CategoryPerformanceInsight
	case "workflow_pattern":
		return CategoryWorkflowPattern
	case "team_preference":
		return CategoryTeamPreference
	default:
		return CategoryUnknown
	}
}

type Confidence int32

const (
	ConfidenceLow    Confidence = 1
	ConfidenceMedium Confidence = 2
	ConfidenceHigh   Confidence = 3
)

// Entry is the main cultural knowledge aggregate.
// It captures a piece of reusable knowledge learned from agent work.
type Entry struct {
	ID        ID
	TenantID  shared.TenantID
	ProjectID shared.ProjectID

	Title    string
	Content  string
	Category Category
	Tags     []string

	SourceAgentID string
	SourceTaskID  string
	Confidence    Confidence
	UsageCount    int32

	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewEntry(
	clock shared.Clock,
	id ID,
	tenantID shared.TenantID,
	projectID shared.ProjectID,
	title, content string,
	category Category,
	tags []string,
	sourceAgentID, sourceTaskID string,
) (*Entry, error) {
	if tenantID == "" {
		return nil, shared.ValidationError{Msg: "tenant_id is required"}
	}
	if projectID == "" {
		return nil, shared.ValidationError{Msg: "project_id is required"}
	}
	if title == "" {
		return nil, shared.ValidationError{Msg: "title is required"}
	}
	if content == "" {
		return nil, shared.ValidationError{Msg: "content is required"}
	}
	if category == CategoryUnknown {
		return nil, shared.ValidationError{Msg: "category is required"}
	}

	now := clock.Now()
	return &Entry{
		ID:            id,
		TenantID:      tenantID,
		ProjectID:     projectID,
		Title:         title,
		Content:       content,
		Category:      category,
		Tags:          tags,
		SourceAgentID: sourceAgentID,
		SourceTaskID:  sourceTaskID,
		Confidence:    ConfidenceMedium,
		UsageCount:    0,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (e *Entry) UpdateContent(clock shared.Clock, title, content string) error {
	if e == nil {
		return shared.ValidationError{Msg: "entry is nil"}
	}
	if title == "" && content == "" {
		return shared.ValidationError{Msg: "title or content required"}
	}
	if title != "" {
		e.Title = title
	}
	if content != "" {
		e.Content = content
	}
	e.UpdatedAt = clock.Now()
	return nil
}

func (e *Entry) SetConfidence(clock shared.Clock, c Confidence) error {
	if e == nil {
		return shared.ValidationError{Msg: "entry is nil"}
	}
	if c < ConfidenceLow || c > ConfidenceHigh {
		return shared.ValidationError{Msg: "confidence must be 1-3"}
	}
	e.Confidence = c
	e.UpdatedAt = clock.Now()
	return nil
}

func (e *Entry) RecordUsage(clock shared.Clock) {
	if e == nil {
		return
	}
	e.UsageCount++
	e.UpdatedAt = clock.Now()
}

func (e *Entry) Deactivate(clock shared.Clock) error {
	if e == nil {
		return shared.ValidationError{Msg: "entry is nil"}
	}
	if !e.Active {
		return nil
	}
	e.Active = false
	e.UpdatedAt = clock.Now()
	return nil
}
