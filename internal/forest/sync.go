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
	line.Commits = len(recorded)

	touched := lastHead == "" // no cursor: cannot prove no-change, scan
	if !touched {
		touched = gateOpens(gitcmd.DiffFiles(ctx, line.Repo.LocalPath, lastHead, head), line.Repo.Lang)
	}
	if touched && line.Repo.Lang == "" {
		// No adapter for this tree: it still grows, it just draws no
		// architecture. Bookkeeping and the cursor move on regardless.
		line.Note = "no adapter for language"
	} else if touched {
		prevID, _ := db.LatestSnapshotID(ctx, line.Repo.ID)
		report, err := scanner.Run(ctx, db, line.Repo.LocalPath)
		if err != nil {
			line.Action, line.Note = "failed", "scan: "+err.Error()
			return // cursor stays: next round retries
		}
		if err := db.SetSnapshotCommit(ctx, report.SnapshotID, head, branch); err != nil {
			line.Action, line.Note = "failed", err.Error()
			return
		}
		// The growth story of this round (growth spec 切片 B): diff the
		// snapshot the cursor pointed at against the fresh one. A failure
		// fails the line before the cursor moves, so the next round redoes
		// the whole step — the event upsert is idempotent. The first scan
		// of a repo (no prev snapshot) records nothing: a baseline is not
		// a growth event.
		if prevID != "" && prevID != report.SnapshotID {
			if err := recordGrowth(ctx, db, line.Repo.ID, prevID, report.SnapshotID, recorded); err != nil {
				line.Action, line.Note = "failed", "growth: "+err.Error()
				return
			}
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
// attribution verdict. The records come back newest first for the growth
// attribution below.
func recordCommits(ctx context.Context, db *store.DB, line *RepoReport, lastHead, head string) ([]model.Commit, error) {
	recs, err := gitcmd.Log(ctx, line.Repo.LocalPath, lastHead, head, maxLogPerRound)
	if err != nil {
		return nil, err
	}
	commits := make([]model.Commit, 0, len(recs))
	for _, r := range recs {
		commits = append(commits, model.Commit{RepoID: line.Repo.ID, Hash: r.Hash,
			Author: r.Author, AuthorEmail: r.AuthorEmail, Date: r.Date, Message: r.Message,
			AgentTag: growth.Attribution(r.Author+" <"+r.AuthorEmail+">", r.Body),
			Files:    r.Files})
	}
	if err := db.SaveCommits(ctx, commits); err != nil {
		return nil, err
	}
	return commits, nil
}

// recordGrowth diffs two snapshots and stores the element-level story with
// commit attribution by file overlap: the newest commit in this round that
// touched the element's file. Edge events stay unattributed on purpose — an
// edge lives in two files, naming one would be a guess (growth spec §6 risk
// 4: attribution is a strong hint, never an adjudication).
func recordGrowth(ctx context.Context, db *store.DB, repoID, from, to string, commits []model.Commit) error {
	prevElements, err := db.Elements(ctx, from)
	if err != nil {
		return err
	}
	prevRels, err := db.Relationships(ctx, from)
	if err != nil {
		return err
	}
	curElements, err := db.Elements(ctx, to)
	if err != nil {
		return err
	}
	curRels, err := db.Relationships(ctx, to)
	if err != nil {
		return err
	}
	events := growth.Diff(prevElements, prevRels, curElements, curRels)
	if len(events) == 0 {
		return nil
	}
	fileOf := map[string]string{}
	for _, e := range prevElements {
		fileOf[e.ID] = e.FilePath
	}
	for _, e := range curElements {
		fileOf[e.ID] = e.FilePath
	}
	for i := range events {
		events[i].RepoID, events[i].SnapshotFrom, events[i].SnapshotTo = repoID, from, to
		if events[i].Kind != model.GrowthEdgeChanged {
			events[i].CommitHash = commitFor(commits, fileOf[events[i].ElementID])
		}
	}
	return db.SaveGrowthEvents(ctx, events)
}

// commitFor returns the hash of the newest commit touching path; commits is
// newest first (gitcmd.Log order), so the first hit wins.
func commitFor(commits []model.Commit, path string) string {
	if path == "" {
		return ""
	}
	for _, c := range commits {
		for _, f := range c.Files {
			if f == path {
				return c.Hash
			}
		}
	}
	return ""
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