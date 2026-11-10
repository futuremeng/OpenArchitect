// Package gitcmd is the forest's only git exit: every git read in the
// observation trunk goes through here via os/exec (I3 style — no SDK, no
// go-git, no cgo).
package gitcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNotRepo marks a directory without git metadata.
var ErrNotRepo = errors.New("gitcmd: not a git repository")

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// IsRepo reports whether dir carries git metadata.
func IsRepo(ctx context.Context, dir string) bool {
	_, err := run(ctx, dir, "rev-parse", "--git-dir")
	return err == nil
}

// IsLinkedWorktree reports whether dir is a `git worktree add` checkout or a
// submodule: its .git is a *file* pointing at the common dir, not a directory.
// Such a dir is the same tree as its main checkout — registering it as its own
// repo would double-count every commit. Orca's worktree layout makes this the
// forest's first toolchain-coordination rule (positioning doc 2026-11-10).
func IsLinkedWorktree(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && !info.IsDir()
}

// Head returns the full HEAD hash and the current branch name.
func Head(ctx context.Context, dir string) (hash, branch string, err error) {
	h, err := run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", "", ErrNotRepo
	}
	b, berr := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if berr != nil {
		b = "HEAD" // detached: rev-parse says so itself
	}
	return strings.TrimSpace(h), strings.TrimSpace(b), nil
}

// RemoteURL returns the origin remote, or "" when there is none (a purely
// local repo is a normal forest citizen).
func RemoteURL(ctx context.Context, dir string) string {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Fetch asks origin for updates. Offline is a normal outcome for a local-first
// tool, so callers log the error but do not fail the sync round.
func Fetch(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "fetch", "--quiet", "origin")
	return err
}

// DiffFiles lists paths changed between two revisions. Any git failure
// (unknown old hash, shallow clone) degrades to "changed everything": the
// safe answer is to rescan (1.0 plan design decision 3).
func DiffFiles(ctx context.Context, dir, from, to string) []string {
	if from == "" {
		return nil // no cursor: caller treats nil as "cannot prove no-change"
	}
	out, err := run(ctx, dir, "diff", "--name-only", from+".."+to)
	if err != nil {
		return []string{"?"}
	}
	return splitLines(out)
}

// Record is one raw commit as git reports it. Message is the subject line;
// Body carries the rest, where Co-authored-by trailers live.
type Record struct {
	Hash, Author, AuthorEmail, Date, Message, Body string
	Files                                          []string
}

const (
	recordSep = "\x1e" // git log --format=%x1e...: one record boundary per commit
	fieldSep  = "\x00"
)

// Log lists commits in (from..to], newest first, with touched files. from
// empty means the last `max` commits. Separators travel as %x escapes:
// literal NUL bytes cannot go through execve. A chunk whose hash field is
// not 40 hex digits is dropped, so no message can forge a record.
func Log(ctx context.Context, dir, from, to string, max int) ([]Record, error) {
	format := "%x1e%H%x00%an%x00%ae%x00%aI%x00%s%x00%b%x01"
	args := []string{"log", "--name-only", "--format=" + format}
	if from != "" {
		args = append(args, from+".."+to)
	} else {
		args = append(args, "-n", itoa(max), to)
	}
	out, err := run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, chunk := range strings.Split(out, recordSep) {
		if chunk == "" {
			continue
		}
		head, filesPart, hasFiles := strings.Cut(chunk, filesMark)
		fields := strings.Split(head, fieldSep)
		if len(fields) < 6 || !isHexHash(fields[0]) {
			continue // forged or truncated chunk: no valid hash, no record
		}
		rec := Record{Hash: fields[0], Author: fields[1], AuthorEmail: fields[2],
			Date: fields[3], Message: fields[4], Body: fields[5]}
		if hasFiles {
			rec.Files = splitLines(filesPart)
		}
		records = append(records, rec)
		if len(records) >= max {
			break
		}
	}
	return records, nil
}

// filesMark ends the formatted header; everything after it in the record is
// the --name-only file block. A blank line cannot be the delimiter because
// commit bodies legitimately contain blank lines.
const filesMark = "\x01"

func isHexHash(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line = filepath.ToSlash(strings.TrimSpace(line)); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func itoa(n int) string {
	if n <= 0 {
		n = 50
	}
	return strconv.Itoa(n)
}