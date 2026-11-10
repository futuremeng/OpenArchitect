// Package growth turns snapshots into a story: what changed between them and
// who changed it. 1.0 ships the attribution half only (plan C-lite); the diff
// engine lands with growth slice B. It imports core only (growth spec §5).
package growth

import (
	"regexp"
	"strings"
)

// AgentTrailers maps "Co-authored-by" identities to agent names. It is data,
// not code paths (growth spec 切片 C): adding a tool means one entry here.
// Patterns match the trailer's name+email, case-insensitively.
var AgentTrailers = []struct {
	Name    string
	Pattern *regexp.Regexp
}{
	{"claude", regexp.MustCompile(`(?i)claude[\s<].*anthropic\.com|(?i)^\s*claude`)},
	{"codex", regexp.MustCompile(`(?i)codex`)},
	{"opencode", regexp.MustCompile(`(?i)opencode`)},
}

var coAuthorRe = regexp.MustCompile(`(?i)^co-authored-by:\s*(.+)$`)

func matchTrailer(line string, pattern *regexp.Regexp) bool {
	m := coAuthorRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return false
	}
	return pattern.MatchString(m[1])
}

// Attribution judges one commit: "human", "agent:<name>", or "mixed" when an
// agent trailer rides on a human author. A trailer is a strong hint, not an
// adjudication (growth spec §6 risk 4).
func Attribution(author, message string) string {
	headAgent := ""
	for _, p := range AgentTrailers {
		if p.Pattern.MatchString(author) {
			headAgent = p.Name
		}
	}
	trailerAgents := map[string]bool{}
	for _, line := range strings.Split(message, "\n") {
		for _, p := range AgentTrailers {
			if matchTrailer(line, p.Pattern) {
				trailerAgents[p.Name] = true
			}
		}
	}
	switch {
	case headAgent != "":
		return "agent:" + headAgent
	case len(trailerAgents) == 0:
		return "human"
	default:
		return "mixed" // human authored, at least one agent co-authored
	}
}