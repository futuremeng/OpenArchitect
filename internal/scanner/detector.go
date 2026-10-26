// Package scanner turns a directory into stored architecture: it picks a
// language adapter, parses, and writes one snapshot in one transaction.
// It imports no UI stack (spec §5.1, R1).
package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/futuremeng/OpenArchitect/internal/scanner/language"
)

var (
	ErrUnsupportedLanguage = errors.New("scanner: unsupported language")
	ErrUnreadable          = errors.New("scanner: cannot read directory")
)

// Detect returns the language name for root. It separates "no adapter
// recognised this project" from "the project could not be read at all",
// because the UI has to say different things about each (spec §10).
func Detect(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrUnsupportedLanguage, root)
	}
	var unreadable error
	for _, adapter := range language.All() {
		ok, err := adapter.Detect(root)
		if err != nil {
			// spec §10 row 2 says a directory without go.mod is "不支持的语言",
			// while "读不了" is a different answer the UI phrases differently.
			// The go adapter folds both causes into one (false, err) by design
			// (Task 9 pins it), so the distinction is recovered here by error
			// identity: fs.ErrNotExist means this adapter simply did not
			// recognise the project, so keep trying the rest; any other error
			// (EACCES on a 0o000 directory, an I/O fault) really is "could not
			// read", and must not be reported as "unsupported".
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			unreadable = err
			continue
		}
		if ok {
			return adapter.Language(), nil
		}
	}
	if unreadable != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreadable, unreadable)
	}
	return "", ErrUnsupportedLanguage
}
