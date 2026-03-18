package knowledge

import (
	"context"

	domainKnowledge "server/internal/domain/knowledge"
	"server/internal/domain/shared"
)

type KnowledgeRepository interface {
	Save(ctx context.Context, entry *domainKnowledge.Entry) error
	GetByID(ctx context.Context, id domainKnowledge.ID) (*domainKnowledge.Entry, error)
	FindByProject(ctx context.Context, projectID shared.ProjectID) ([]*domainKnowledge.Entry, error)
	FindByCategory(ctx context.Context, projectID shared.ProjectID, category domainKnowledge.Category) ([]*domainKnowledge.Entry, error)
	SearchByTags(ctx context.Context, projectID shared.ProjectID, tags []string) ([]*domainKnowledge.Entry, error)
	SearchByContent(ctx context.Context, projectID shared.ProjectID, query string) ([]*domainKnowledge.Entry, error)
	Delete(ctx context.Context, id domainKnowledge.ID) error
}
