package domain

// SkeletonSpec describes the project structure for deterministic skeleton creation.
// Created by Claude on the server, sent to proxy via ActionCreateSkeleton.
type SkeletonSpec struct {
	Module struct {
		Type string `json:"type"` // "go", "node", "python"
		Name string `json:"name"` // module/package name
	} `json:"module"`
	Files []SkeletonFile `json:"files"`
	ProjectName string `json:"projectName"`
	ProjectGoal string `json:"projectGoal"`
}

type SkeletonFile struct {
	Path    string `json:"path"`    // relative path from project root
	Purpose string `json:"purpose"` // single-line description
}
