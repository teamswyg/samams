package project

import (
	"encoding/json"
	"strings"
)

// NodeType constants for the 3-level tree hierarchy.
const (
	NodeTypeProposal  = "proposal"
	NodeTypeMilestone = "milestone"
	NodeTypeTask      = "task"
)

// treeNode is an intermediate representation for parsing/fixing LLM-generated node trees.
type treeNode struct {
	ID              string `json:"id"`
	UID             string `json:"uid"`
	Type            string `json:"type"`
	Summary         string `json:"summary"`
	Agent           string `json:"agent"`
	Status          string `json:"status"`
	Priority        string `json:"priority"`
	ParentID        *string `json:"parentId"`
	BoundedContext  string `json:"boundedContext"`
	EstimatedTokens int    `json:"estimatedTokens,omitempty"`
}

// EnforceTreeHierarchy parses LLM-generated JSON and enforces the 3-level type rule:
//   - parentId=null        → proposal
//   - parent is proposal   → milestone
//   - parent is milestone  → task
//
// Returns the corrected JSON string.
func EnforceTreeHierarchy(raw string) (string, error) {
	// Strip markdown code fences that LLMs often wrap around JSON.
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		cleaned = strings.TrimSuffix(strings.TrimSpace(cleaned), "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	var wrapper struct {
		Nodes []treeNode `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(cleaned), &wrapper); err != nil {
		return raw, err
	}

	// Build parent lookup: id → node index.
	byID := make(map[string]int, len(wrapper.Nodes))
	for i, n := range wrapper.Nodes {
		byID[n.ID] = i
	}

	// Determine depth for each node and assign type.
	for i := range wrapper.Nodes {
		depth := nodeDepth(wrapper.Nodes, byID, i)
		switch depth {
		case 0:
			wrapper.Nodes[i].Type = NodeTypeProposal
		case 1:
			wrapper.Nodes[i].Type = NodeTypeMilestone
		default:
			wrapper.Nodes[i].Type = NodeTypeTask
		}
	}

	out, err := json.Marshal(wrapper)
	if err != nil {
		return raw, err
	}
	return string(out), nil
}

// nodeDepth returns the depth of the node in the tree (0 = root).
func nodeDepth(nodes []treeNode, byID map[string]int, idx int) int {
	depth := 0
	cur := idx
	for {
		pid := nodes[cur].ParentID
		if pid == nil || *pid == "" {
			return depth
		}
		parent, ok := byID[*pid]
		if !ok {
			return depth
		}
		depth++
		cur = parent
		if depth > 10 { // safety guard against cycles
			return depth
		}
	}
}
