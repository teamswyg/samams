package port

import "proxy/internal/domain"

// BranchManager is a secondary (driven) port for git branch operations.
// Uses git worktrees: each branch gets an isolated working directory.
//
// Workspace structure:
//
//	~/.samams/workspaces/{projectName}/{branchDir}/
type BranchManager interface {
	// InitWorkspace creates the project workspace: workspaces/{projectName}/main/ + git init.
	// Returns the main folder path. Idempotent — returns existing path if already initialized.
	InitWorkspace(projectName string) (mainPath string, err error)

	// CreateSkeleton creates the project workspace with a deterministic folder structure.
	// Replaces the proposal agent: proxy creates skeleton directly from SkeletonSpec JSON.
	CreateSkeleton(spec domain.SkeletonSpec) (mainPath string, err error)

	// CreateWorktree creates a git worktree for the branch and returns its path.
	CreateWorktree(projectName, branchName, baseBranch string) (worktreePath string, err error)

	// MergeBack merges the child branch into the parent, then removes the worktree.
	MergeBack(childBranch, parentBranch string) error

	// RemoveWorktree removes a worktree directory and optionally deletes the branch.
	RemoveWorktree(branchName string) error

	// WorktreePath returns the worktree path for a branch (empty if not exists).
	WorktreePath(branchName string) string

	// ResetWorktree resets a task worktree to HEAD and cleans untracked files.
	// Only allowed on dev/TASK-* branches for safety.
	ResetWorktree(branchName string) error

	// PurgeWorktree removes the worktree and force-deletes the branch.
	// Only allowed on dev/TASK-* branches for safety.
	PurgeWorktree(branchName string) error

	// Close drains the merge queue and stops. Call on shutdown.
	Close()
}
