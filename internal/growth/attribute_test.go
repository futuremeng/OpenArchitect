package growth

import "testing"

func TestAttribution(t *testing.T) {
	cases := []struct {
		name, author, message, want string
	}{
		{"plain human", "Alice <alice@example.com>", "feat: add thing", "human"},
		{"agent authored", "Claude <noreply@anthropic.com>", "fix: thing", "agent:claude"},
		{"codex authored", "Codex <codex@openai.com>", "fix: thing", "agent:codex"},
		{"claude trailer", "Alice <alice@example.com>",
			"feat: x\n\nCo-Authored-By: Claude <noreply@anthropic.com>", "mixed"},
		{"opencode trailer", "Alice <alice@example.com>",
			"feat: x\n\nCo-authored-by: opencode <bot@opencode.ai>", "mixed"},
		{"human trailer is human", "Alice <alice@example.com>",
			"feat: x\n\nCo-authored-by: Bob <bob@example.com>", "human"},
		{"trailer line in body counts", "Alice <a@x>",
			"Co-Authored-By: Claude <noreply@anthropic.com>", "mixed"},
		{"agent author wins over co-author", "Claude <noreply@anthropic.com>",
			"feat: x\n\nCo-authored-by: Bob <bob@example.com>", "agent:claude"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Attribution(c.author, c.message); got != c.want {
				t.Fatalf("Attribution(%q, %q) = %q, want %q", c.author, c.message, got, c.want)
			}
		})
	}
}