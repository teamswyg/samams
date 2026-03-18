package domain

import "strings"

type NodeType string

const (
	NodeProposal  NodeType = "proposal"
	NodeMilestone NodeType = "milestone"
	NodeTask      NodeType = "task"
)

// BranchName returns the branch name for a tree-node.
// Proposal → "" (uses main). Milestones/Tasks get their own branches.
func BranchName(nodeType, uid, summary string) string {
	if NodeType(nodeType) == NodeProposal {
		return ""
	}
	prefix := inferPrefix(NodeType(nodeType), summary)
	return prefix + uid
}

// ParentBranchName returns the branch name of a node's parent.
// Proposal creates main → milestones fork from main → tasks fork from milestones.
func ParentBranchName(parentType, parentUID, parentSummary string) string {
	if parentUID == "" || NodeType(parentType) == NodeProposal {
		return "main"
	}
	return BranchName(parentType, parentUID, parentSummary)
}

func inferPrefix(nodeType NodeType, summary string) string {
	switch nodeType {
	case NodeProposal:
		return ""
	case NodeMilestone:
		return "dev/"
	case NodeTask:
		lower := strings.ToLower(summary)
		switch {
		case containsAny(lower, "hotfix", "urgent fix", "critical fix"):
			return "hotfix/"
		case containsAny(lower, "bug", "fix", "patch", "repair", "resolve"):
			return "fix/"
		default:
			return "dev/"
		}
	default:
		return "dev/"
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
