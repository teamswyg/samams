// Package gitbranch implements port.BranchManager using git worktrees.
//
// Each project gets an isolated workspace under ~/.samams/workspaces/{projectName}/.
// The main/ folder is the git repo (git init). Worktrees are sibling directories.
// Agents work exclusively in their worktree — file access is naturally scoped.
//
// Structure:
//
//	~/.samams/workspaces/{projectName}/
//	  main/                                 ← repoDir (git init, .git lives here)
//	  dev-MLST-0001-A/                      ← milestone worktree
//	  dev-TASK-0001-1/                      ← task worktree (agent works here)
//
// Lifecycle:
//  1. CreateSkeleton(spec) or InitWorkspace(projectName) → creates main/ + skeleton + git init
//  2. CreateWorktree(projectName, branchName, baseBranch) → git worktree add
//  3. Agent works in worktree directory (cmd.Dir = worktree path)
//  4. MergeBack(childBranch, parentBranch) → FIFO merge queue → merge + cleanup
package gitbranch

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"proxy/internal/domain"
)

// ── Merge Queue (FIFO) ──────────────────────────────────────────

type mergeRequest struct {
	ChildBranch  string
	ParentBranch string
	DoneCh       chan error
}

type mergeQueue struct {
	ch   chan mergeRequest
	done chan struct{}
}

func newMergeQueue() *mergeQueue {
	return &mergeQueue{
		ch:   make(chan mergeRequest, 20),
		done: make(chan struct{}),
	}
}

func (q *mergeQueue) run(m *Manager) {
	defer close(q.done)
	for req := range q.ch {
		err := m.doMerge(req.ChildBranch, req.ParentBranch)
		req.DoneCh <- err
	}
	// Drain any remaining requests after channel close (notify callers).
	for {
		select {
		case req, ok := <-q.ch:
			if !ok {
				return
			}
			req.DoneCh <- fmt.Errorf("merge queue shutting down")
		default:
			return
		}
	}
}

// ── Manager ─────────────────────────────────────────────────────

// Manager handles git worktree operations for task isolation.
type Manager struct {
	repoDir      string
	worktreeRoot string

	mu        sync.Mutex
	worktrees map[string]string // branchName → worktree path

	mq *mergeQueue
}

func New(repoDir string) *Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home, _ = os.Getwd()
		log.Printf("[worktree] Warning: UserHomeDir failed, using cwd: %v", err)
	}
	root := filepath.Join(home, ".samams", "workspaces")
	os.MkdirAll(root, 0755)

	m := &Manager{
		repoDir:      repoDir,
		worktreeRoot: root,
		worktrees:    make(map[string]string),
		mq:           newMergeQueue(),
	}
	go m.mq.run(m)
	return m
}

// Close drains the merge queue and stops. Call on shutdown.
func (m *Manager) Close() {
	close(m.mq.ch)
	<-m.mq.done
	log.Println("[worktree] Merge queue drained, manager closed")
}

// ── Skeleton Creation ───────────────────────────────────────────

// CreateSkeleton creates the project workspace with a deterministic folder structure.
// Replaces the proposal agent: proxy creates skeleton directly from SkeletonSpec JSON.
func (m *Manager) CreateSkeleton(spec domain.SkeletonSpec) (string, error) {
	start := time.Now()

	// InitWorkspace acquires its own lock, so call it first.
	mainPath, err := m.InitWorkspace(spec.ProjectName)
	if err != nil {
		return "", fmt.Errorf("init workspace: %w", err)
	}

	// Now hold lock for skeleton file creation (no nested lock — InitWorkspace already released).
	m.mu.Lock()
	defer m.mu.Unlock()

	fileCount := 0
	for _, f := range spec.Files {
		fullPath := filepath.Join(mainPath, filepath.FromSlash(f.Path))

		if strings.HasSuffix(f.Path, "/") {
			// Directory only.
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				return "", fmt.Errorf("mkdir %s: %w", f.Path, err)
			}
			continue
		}

		// File: create parent dir + file with purpose comment.
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("mkdir for %s: %w", f.Path, err)
		}

		content := fileComment(spec.Module.Type, f.Purpose, f.Path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write %s: %w", f.Path, err)
		}
		fileCount++
	}

	// Module file.
	switch spec.Module.Type {
	case "go":
		goMod := fmt.Sprintf("module %s\n\ngo 1.26.1\n", spec.Module.Name)
		if err := os.WriteFile(filepath.Join(mainPath, "go.mod"), []byte(goMod), 0644); err != nil {
			return "", fmt.Errorf("write go.mod: %w", err)
		}
	case "node":
		pkgJSON := fmt.Sprintf(`{"name": "%s", "version": "0.1.0", "private": true}`, spec.Module.Name)
		if err := os.WriteFile(filepath.Join(mainPath, "package.json"), []byte(pkgJSON), 0644); err != nil {
			return "", fmt.Errorf("write package.json: %w", err)
		}
	case "python":
		content := fmt.Sprintf("[project]\nname = \"%s\"\nversion = \"0.1.0\"\n", spec.Module.Name)
		if err := os.WriteFile(filepath.Join(mainPath, "pyproject.toml"), []byte(content), 0644); err != nil {
			return "", fmt.Errorf("write pyproject.toml: %w", err)
		}
	}

	// README.
	readme := fmt.Sprintf("# %s\n\n%s\n", spec.ProjectName, spec.ProjectGoal)
	if err := os.WriteFile(filepath.Join(mainPath, "README.md"), []byte(readme), 0644); err != nil {
		return "", fmt.Errorf("write README.md: %w", err)
	}

	// .gitignore — exclude SAMAMS agent artifacts that cause merge conflicts.
	gitignore := ".samams-context.md\n.samams-prompt.md\n"
	if err := os.WriteFile(filepath.Join(mainPath, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return "", fmt.Errorf("write .gitignore: %w", err)
	}

	// Git commit.
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = mainPath
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "initial project skeleton")
	commitCmd.Dir = mainPath
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	log.Printf("[skeleton] Created %d files in %dms (%s)", fileCount, time.Since(start).Milliseconds(), spec.ProjectName)
	return mainPath, nil
}

// fileComment returns a single-line placeholder comment for the given language.
// For Go files, derives the package name from the file's directory.
func fileComment(moduleType, purpose, filePath string) string {
	switch moduleType {
	case "go":
		pkgName := filepath.Base(filepath.Dir(filePath))
		if pkgName == "." || pkgName == "" || pkgName == "/" {
			pkgName = "main"
		}
		return "// " + purpose + "\npackage " + pkgName + "\n"
	case "node", "typescript":
		return "// " + purpose + "\n"
	case "python":
		return "# " + purpose + "\n"
	default:
		return "// " + purpose + "\n"
	}
}

// ── Workspace Init ──────────────────────────────────────────────

// InitWorkspace creates the project workspace: workspaces/{projectName}/main/ + git init.
func (m *Manager) InitWorkspace(projectName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	projDir := sanitizeDirName(projectName)
	if projDir == "" {
		projDir = "default"
	}
	mainPath := filepath.Join(m.worktreeRoot, projDir, "main")

	// If already initialized, return existing path.
	if _, err := os.Stat(filepath.Join(mainPath, ".git")); err == nil {
		m.repoDir = mainPath
		log.Printf("[workspace] Reusing existing workspace: %s", mainPath)
		return mainPath, nil
	}

	// Create and init.
	if err := os.MkdirAll(mainPath, 0755); err != nil {
		return "", fmt.Errorf("create main dir: %w", err)
	}

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = mainPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git init: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	// Create initial .gitkeep so we have something to commit.
	if err := os.WriteFile(filepath.Join(mainPath, ".gitkeep"), []byte(""), 0644); err != nil {
		return "", fmt.Errorf("create .gitkeep: %w", err)
	}

	initCmd := exec.Command("git", "add", ".")
	initCmd.Dir = mainPath
	if out, err := initCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "initial workspace setup")
	commitCmd.Dir = mainPath
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	m.repoDir = mainPath
	log.Printf("[workspace] Initialized: %s (project: %s)", mainPath, projectName)
	return mainPath, nil
}

// ── Worktree Operations ─────────────────────────────────────────

// CreateWorktree creates an isolated worktree for the given branch.
func (m *Manager) CreateWorktree(projectName, branchName, baseBranch string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	projDir := sanitizeDirName(projectName)
	if projDir == "" {
		projDir = "default"
	}
	dirName := strings.ReplaceAll(branchName, "/", "-")
	wtPath := filepath.Join(m.worktreeRoot, projDir, dirName)

	// If worktree already exists, return its path.
	if _, err := os.Stat(wtPath); err == nil {
		m.worktrees[branchName] = wtPath
		log.Printf("[worktree] Reusing existing worktree: %s → %s", branchName, wtPath)
		return wtPath, nil
	}

	if err := os.MkdirAll(m.worktreeRoot, 0755); err != nil {
		return "", fmt.Errorf("create worktree root: %w", err)
	}

	// Ensure base branch exists + has a worktree.
	// Milestone branches are lazy-initialized: branch + worktree created on first child task.
	if baseBranch != "main" && !m.branchExists(baseBranch) {
		if err := m.git("branch", baseBranch, "main"); err != nil {
			log.Printf("[worktree] Warning: auto-create base branch %s from main failed: %v", baseBranch, err)
		} else {
			log.Printf("[worktree] Auto-created milestone branch %s from main", baseBranch)
		}
	}
	// Create milestone worktree if it doesn't exist (needed for review agent + merge target).
	if baseBranch != "main" {
		dirName := strings.ReplaceAll(baseBranch, "/", "-")
		mwtPath := filepath.Join(m.worktreeRoot, projDir, dirName)
		if _, err := os.Stat(mwtPath); os.IsNotExist(err) {
			if err := m.git("worktree", "add", mwtPath, baseBranch); err != nil {
				log.Printf("[worktree] Warning: milestone worktree creation failed for %s: %v", baseBranch, err)
			} else {
				m.worktrees[baseBranch] = mwtPath
				log.Printf("[worktree] Created milestone worktree: %s → %s", baseBranch, mwtPath)
			}
		} else if _, ok := m.worktrees[baseBranch]; !ok {
			m.worktrees[baseBranch] = mwtPath
		}
	}

	// Create branch from base if it doesn't exist.
	if !m.branchExists(branchName) {
		if err := m.git("branch", branchName, baseBranch); err != nil {
			log.Printf("[worktree] Branch create hint: %v (may already exist)", err)
		}
	}

	// Create worktree.
	if err := m.git("worktree", "add", wtPath, branchName); err != nil {
		return "", fmt.Errorf("worktree add %s: %w", branchName, err)
	}

	m.worktrees[branchName] = wtPath
	log.Printf("[worktree] Created: %s → %s (base: %s)", branchName, wtPath, baseBranch)
	return wtPath, nil
}

// ── Merge (FIFO Queue) ─────────────────────────────────────────

// MergeBack submits a merge request to the FIFO queue and waits for completion.
// Guarantees FIFO ordering — earlier submissions merge first.
func (m *Manager) MergeBack(childBranch, parentBranch string) error {
	doneCh := make(chan error, 1)
	m.mq.ch <- mergeRequest{
		ChildBranch:  childBranch,
		ParentBranch: parentBranch,
		DoneCh:       doneCh,
	}
	log.Printf("[merge-queue] Queued: %s → %s", childBranch, parentBranch)
	return <-doneCh
}

// doMerge performs the actual merge. Called by the queue worker goroutine.
// If parentBranch has a worktree (milestone), merge INSIDE that worktree.
// Otherwise, checkout parent in main repo and merge there.
func (m *Manager) doMerge(childBranch, parentBranch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("[merge-queue] Processing: %s → %s", childBranch, parentBranch)

	// Pre-merge: remove .samams-* agent artifacts from child branch to prevent conflicts.
	// These files are different in every branch and always cause CONFLICT on merge.
	m.cleanAgentArtifacts(childBranch)

	if wtPath, ok := m.worktrees[parentBranch]; ok {
		// Also clean parent worktree artifacts.
		m.cleanAgentArtifactsInDir(wtPath, parentBranch)

		// Rebase child onto parent to incorporate changes from earlier merges.
		// This auto-resolves simple conflicts (e.g., add/add in go.mod) that
		// arise when parallel tasks create the same files independently.
		if childWT, hasWT := m.worktrees[childBranch]; hasWT {
			if err := m.gitIn(childWT, "rebase", parentBranch); err != nil {
				_ = m.gitIn(childWT, "rebase", "--abort")
				log.Printf("[merge-queue] Rebase %s onto %s failed, falling back to merge", childBranch, parentBranch)
			}
		}

		// Parent is a milestone with a worktree — merge inside it.
		if err := m.gitIn(wtPath, "merge", "--no-ff", childBranch, "-m",
			fmt.Sprintf("merge %s into %s", childBranch, parentBranch)); err != nil {
			_ = m.gitIn(wtPath, "merge", "--abort")
			return fmt.Errorf("merge %s → %s (aborted): %w", childBranch, parentBranch, err)
		}
	} else {
		// Standard: checkout parent in main repo, then merge.
		if err := m.git("checkout", parentBranch); err != nil {
			return fmt.Errorf("checkout parent %s: %w", parentBranch, err)
		}
		if err := m.git("merge", "--no-ff", childBranch, "-m",
			fmt.Sprintf("merge %s into %s", childBranch, parentBranch)); err != nil {
			_ = m.git("merge", "--abort")
			return fmt.Errorf("merge %s → %s (aborted): %w", childBranch, parentBranch, err)
		}
	}

	log.Printf("[merge-queue] Merged %s → %s", childBranch, parentBranch)

	// Keep child worktree alive — watch agents need it for strategy meetings.
	// Cleanup happens when milestone review is approved (via PurgeWorktree).
	return nil
}

// cleanAgentArtifacts removes .samams-* files from a branch's worktree (if it has one).
func (m *Manager) cleanAgentArtifacts(branchName string) {
	wtPath, ok := m.worktrees[branchName]
	if !ok {
		return
	}
	m.cleanAgentArtifactsInDir(wtPath, branchName)
}

// cleanAgentArtifactsInDir removes .samams-* files from a directory and commits the removal.
func (m *Manager) cleanAgentArtifactsInDir(dir, branchName string) {
	removed := false
	for _, name := range []string{".samams-context.md", ".samams-prompt.md"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			// git rm (removes from index + disk). Ignore errors if not tracked.
			_ = m.gitIn(dir, "rm", "-f", "--ignore-unmatch", name)
			os.Remove(path) // also remove if untracked
			removed = true
		}
	}
	if removed {
		// Commit the removal so it doesn't interfere with merge.
		_ = m.gitIn(dir, "add", "-A")
		_ = m.gitIn(dir, "commit", "--allow-empty", "-m", "chore: remove agent artifacts before merge")
		log.Printf("[merge-queue] Cleaned agent artifacts from %s", branchName)
	}
}

// gitIn runs a git command in the specified directory.
func (m *Manager) gitIn(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ── Worktree Reset ──────────────────────────────────────────────

// ResetWorktree resets a task worktree to HEAD and cleans untracked files.
// Only allowed on task branches (dev/|fix/|hotfix/ with TASK- keyword) — main/milestone branches are never reset.
func (m *Manager) ResetWorktree(branchName string) error {
	if !isTaskBranch(branchName) {
		return fmt.Errorf("reset rejected: only task branches (dev/|fix/|hotfix/ + TASK-) allowed, got %q", branchName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	wtPath, ok := m.worktrees[branchName]
	if !ok {
		return fmt.Errorf("worktree not found for branch %s", branchName)
	}

	if err := m.gitIn(wtPath, "reset", "--hard", "HEAD"); err != nil {
		return fmt.Errorf("git reset HEAD in %s: %w", branchName, err)
	}

	if err := m.gitIn(wtPath, "clean", "-fd"); err != nil {
		return fmt.Errorf("git clean in %s: %w", branchName, err)
	}

	log.Printf("[worktree] Reset %s to HEAD + cleaned", branchName)
	return nil
}

// PurgeWorktree removes the worktree and force-deletes the branch.
// Only allowed on task branches (dev/|fix/|hotfix/ with TASK- keyword).
func (m *Manager) PurgeWorktree(branchName string) error {
	if !isTaskBranch(branchName) {
		return fmt.Errorf("purge rejected: only task branches (dev/|fix/|hotfix/ + TASK-) allowed, got %q", branchName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	wtPath, ok := m.worktrees[branchName]
	if !ok {
		dirName := strings.ReplaceAll(branchName, "/", "-")
		if m.repoDir != "" {
			wtPath = filepath.Join(filepath.Dir(m.repoDir), dirName)
		} else {
			wtPath = filepath.Join(m.worktreeRoot, dirName)
		}
	}

	if err := m.git("worktree", "remove", "--force", wtPath); err != nil {
		log.Printf("[worktree] Purge worktree %s: %v (attempting os remove)", branchName, err)
		os.RemoveAll(wtPath)
	}

	// Force delete branch (allows unmerged).
	if err := m.git("branch", "-D", branchName); err != nil {
		log.Printf("[worktree] Purge branch %s: %v", branchName, err)
	}

	delete(m.worktrees, branchName)
	log.Printf("[worktree] Purged: %s (worktree + branch)", branchName)
	return nil
}

// ── Cleanup ─────────────────────────────────────────────────────

func (m *Manager) RemoveWorktree(branchName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeWorktreeLocked(branchName)
	return nil
}

func (m *Manager) WorktreePath(branchName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check in-memory map first.
	if path, ok := m.worktrees[branchName]; ok {
		return path
	}

	// Fallback: check disk (worktree may exist from a previous session).
	dirName := strings.ReplaceAll(branchName, "/", "-")
	entries, _ := os.ReadDir(m.worktreeRoot)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(m.worktreeRoot, e.Name(), dirName)
		if _, err := os.Stat(candidate); err == nil {
			m.worktrees[branchName] = candidate
			// Also recover repoDir if empty (proxy restarted).
			if m.repoDir == "" {
				mainPath := filepath.Join(m.worktreeRoot, e.Name(), "main")
				if _, err := os.Stat(filepath.Join(mainPath, ".git")); err == nil {
					m.repoDir = mainPath
					log.Printf("[worktree] Recovered repoDir from disk: %s", mainPath)
				}
			}
			log.Printf("[worktree] Recovered worktree from disk: %s → %s", branchName, candidate)
			return candidate
		}
	}
	return ""
}

func (m *Manager) removeWorktreeLocked(branchName string) {
	wtPath, ok := m.worktrees[branchName]
	if !ok {
		// Derive project dir from repoDir (e.g., .../workspaces/{project}/main → .../workspaces/{project}).
		dirName := strings.ReplaceAll(branchName, "/", "-")
		if m.repoDir != "" {
			wtPath = filepath.Join(filepath.Dir(m.repoDir), dirName)
		} else {
			wtPath = filepath.Join(m.worktreeRoot, dirName)
		}
	}

	if err := m.git("worktree", "remove", "--force", wtPath); err != nil {
		log.Printf("[worktree] Remove worktree %s: %v (may already be removed)", branchName, err)
		os.RemoveAll(wtPath)
	}

	if err := m.git("branch", "-D", branchName); err != nil {
		log.Printf("[worktree] Delete branch %s: %v", branchName, err)
	}

	delete(m.worktrees, branchName)
	log.Printf("[worktree] Cleaned up: %s", branchName)
}

// ── Helpers ─────────────────────────────────────────────────────

func (m *Manager) branchExists(name string) bool {
	return m.git("rev-parse", "--verify", name) == nil
}

func sanitizeDirName(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	result := strings.TrimRight(b.String(), "-")
	if len(result) > 60 {
		result = result[:60]
		result = strings.TrimRight(result, "-")
	}
	return result
}

// isTaskBranch returns true for branches like dev/TASK-*, fix/TASK-*, hotfix/TASK-*.
// Prevents accidental reset/purge of main or milestone branches.
func isTaskBranch(name string) bool {
	if !strings.Contains(name, "TASK-") {
		return false
	}
	return strings.HasPrefix(name, "dev/") ||
		strings.HasPrefix(name, "fix/") ||
		strings.HasPrefix(name, "hotfix/")
}

func (m *Manager) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}
