// Package orchestrator decomposes global tasks into subtasks and distributes
// them across agents using their MCP-backed skills.
package orchestrator

import (
	"sort"
	"strings"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// SubtaskSpec is a decomposed unit of work, optionally bound to an agent.
type SubtaskSpec struct {
	AgentID     string
	Description string
	Skills      []store.Skill
	Score       int
}

// Decomposer turns a natural-language task into ordered SubtaskSpecs using
// keyword/skill overlap matching.
type Decomposer struct{}

// Decompose matches the task text against each enabled agent's skills. Each
// agent with at least one matching skill yields a subtask (the agent with the
// highest overlap for those keywords). If no agent matches, a single generic
// subtask is produced with an empty AgentID.
func (d Decomposer) Decompose(text string, agents []store.Agent) []SubtaskSpec {
	lower := strings.ToLower(text)
	var specs []SubtaskSpec
	for _, a := range agents {
		if !a.Enabled {
			continue
		}
		var matched []store.Skill
		for _, s := range a.Skills {
			if keywordMatch(lower, s.Name) || keywordMatch(lower, s.Tool) {
				matched = append(matched, s)
			}
		}
		if len(matched) == 0 {
			continue
		}
		specs = append(specs, SubtaskSpec{
			AgentID:     a.ID,
			Description: buildDescription(text, matched),
			Skills:      matched,
			Score:       len(matched),
		})
	}

	sort.SliceStable(specs, func(i, j int) bool {
		if specs[i].Score != specs[j].Score {
			return specs[i].Score > specs[j].Score
		}
		return specs[i].AgentID < specs[j].AgentID
	})

	if len(specs) == 0 {
		return []SubtaskSpec{{Description: text}}
	}
	return specs
}

// keywordMatch reports whether keyword appears (case-insensitively) as a
// substring of the lower-cased task text. Very short keywords are ignored to
// avoid spurious matches.
func keywordMatch(lowerText, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if len(keyword) < 2 {
		return false
	}
	return strings.Contains(lowerText, keyword)
}

func buildDescription(text string, skills []store.Skill) string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return "process: " + text + " (skills: " + strings.Join(names, ", ") + ")"
}
