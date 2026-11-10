package forest

import (
	"context"
	"strings"

	"github.com/futuremeng/OpenArchitect/internal/core/model"
	"github.com/futuremeng/OpenArchitect/internal/core/store"
	"github.com/futuremeng/OpenArchitect/internal/forest/gitcmd"
	"github.com/futuremeng/OpenArchitect/internal/growth"
	"github.com/futuremeng/OpenArchitect/internal/scanner"
)

// maxLogPerRound caps how many commits one sync round reads from a repo that
// has no cursor yet (first sight of an old repo is not a history dump).
const maxLogPerRound = 200

// RepoReport is one repo's line in a sync round; Action is clean/synced/failed.
type RepoReport struct {
	Repo    model.Repo
	Action  string
	From    string // cursor before this round
	To      string // HEAD now
	Commits int    // commits recorded
	Scanned bool   // whether a rescan ran (gate open)
	Note    string // fetch failures and similar non-fatal noise
}

// Report is one whole sync round.
type Report struct {
	Repos []RepoReport
}

// Sync discovers, then for every repo: compares HEAD with the cursor; on
// drift it fetches (best effort), records commits with attribution, applies
// the source-touch gate, rescans only when the gate opens, and advances the
// cursor. These are the only two automated actions I2′ permits: fetch and
// scan. A per-repo failure is reported in that repo's line, never fatal for
// the round.
func Sync(ctx context.Context, db *store.DB, root string) (Report, error) {
	repos, err := Discover(ctx, db, root)
	if err != nil {
		return Report{}, err
	}
	out := Report{}
	for _, repo := range repos {
		line := RepoReport{Repo: repo, Action: "clean"}
		syncOne(ctx, db, &line)
		out.Repos = append(out.Repos, line)
	}
	return out, nil
}

func syncOne(ctx context.Context, db *store.DB, line *RepoReport) {
	head, branch, err := gitcmd.Head(ctx, line.Repo.LocalPath)
	if err != nil {
		line.Action, line.Note = "failed", err.Error()
		return
	}
	line.To = head
	remote, _, lastHead, err := db.RepoSync(ctx, line.Repo.ID)
	if err != nil {
		line.Action, line.Note = "failed", err.Error()
		return
	}
	if lastHead == "" { // fall back to the newest snapshot's bound commit
		lastHead = snapshotCursor(ctx, db, line.Repo.ID)
	}
	line.From = lastHead
	if head == lastHead {
		return
	}
	if ferr := gitcmd.Fetch(ctx, line.Repo.LocalPath); ferr != nil {
		line.Note = "fetch: " + ferr.Error() // offline is a normal outcome
	}
	recorded, err := recordCommits(ctx, db, line, lastHead, head)
	if err != nil {
		line.Action, line.Note = "failed", err.Error()
		return
	}
	line.Commits = recorded

	touched := lastHead == "" // no cursor: cannot prove no-change, scan
	if !touched {
		touched = gateOpens(gitcmd.DiffFiles(ctx, line.Repo.LocalPath, lastHead, head), line.Repo.Lang)
	}
	if touched && line.Repo.Lang == "" {
		// No adapter for this tree: it still grows, it just draws no
		// architecture. Bookkeeping and the cursor move on regardless.
		line.Note = "no adapter for language"
	} else if touched {
		report, err := scanner.Run(ctx, db, line.Repo.LocalPath)
		if err != nil {
			line.Action, line.Note = "failed", "scan: "+err.Error()
			return // cursor stays: next round retries
		}
		if err := db.SetSnapshotCommit(ctx, report.SnapshotID, head, branch); err != nil {
			line.Action, line.Note = "failed", err.Error()
			return
		}
		line.Scanned = true
	}
	if err := db.UpsertRepoSync(ctx, line.Repo.ID, remote, "tracked", head); err != nil {
		line.Action, line.Note = "failed", err.Error()
		return
	}
	line.Action = "synced"
	line.Repo.SyncState = "tracked"
}

// recordCommits reads (lastHead..head] and stores each commit with its
// attribution verdict.
func recordCommits(ctx context.Context, db *store.DB, line *RepoReport, lastHead, head string) (int, error) {
	recs, err := gitcmd.Log(ctx, line.Repo.LocalPath, lastHead, head, maxLogPerRound)
	if err != nil {
		return 0, err
	}
	commits := make([]model.Commit, 0, len(recs))
	for _, r := range recs {
		commits = append(commits, model.Commit{RepoID: line.Repo.ID, Hash: r.Hash,
			Author: r.Author, AuthorEmail: r.AuthorEmail, Date: r.Date, Message: r.Message,
			AgentTag: growth.Attribution(r.Author+" <"+r.AuthorEmail+">", r.Body),
			Files:    r.Files})
	}
	if err := db.SaveCommits(ctx, commits); err != nil {
		return 0, err
	}
	return len(commits), nil
}

// snapshotCursor is the fallback cursor: the newest snapshot's bound commit.
func snapshotCursor(ctx context.Context, db *store.DB, repoID string) string {
	id, err := db.LatestSnapshotID(ctx, repoID)
	if err != nil || id == "" {
		return ""
	}
	snap, err := db.Snapshot(ctx, id)
	if err != nil {
		return ""
	}
	return snap.CommitHash
}

// sourceExts maps the 1.0 languages to the extensions that open the rescan
// gate (1.0 plan design decision 3: bookkeeping-only commits never rescan).
var sourceExts = map[string][]string{"go": {".go", "go.mod", "go.sum"}}

func gateOpens(files []string, lang string) bool {
	exts, ok := sourceExts[lang]
	if !ok { // unknown language: no gate, always rescan
		return true
	}
	for _, f := range files {
		if f == "?" { // git diff failed: safe answer is rescan
			return true
		}
		for _, ext := range exts {
			if strings.HasSuffix(f, ext) {
				return true
			}
		}
	}
	return false
}