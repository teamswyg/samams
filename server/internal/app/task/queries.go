package task

import domainShared "server/internal/domain/shared"

type GetTaskQuery struct {
	TaskID domainShared.TaskID
}

type ListProjectTasksQuery struct {
	TenantID  domainShared.TenantID
	ProjectID domainShared.ProjectID
}

