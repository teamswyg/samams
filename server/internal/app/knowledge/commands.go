package knowledge

import (
	domainKnowledge "server/internal/domain/knowledge"
	"server/internal/domain/shared"
)

type CreateEntryCommand struct {
	TenantID      shared.TenantID
	ProjectID     shared.ProjectID
	Title         string
	Content       string
	Category      domainKnowledge.Category
	Tags          []string
	SourceAgentID string
	SourceTaskID  string
}

type UpdateEntryCommand struct {
	ID      domainKnowledge.ID
	Title   string
	Content string
}

type SetConfidenceCommand struct {
	ID         domainKnowledge.ID
	Confidence domainKnowledge.Confidence
}

type SearchCommand struct {
	ProjectID shared.ProjectID
	Query     string
	Category  domainKnowledge.Category
	Tags      []string
}
