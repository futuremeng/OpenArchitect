package model

// Repo keeps only fields this slice has a source for: URL and Branch were
// dropped deliberately (spec deviation 9).
type Repo struct {
	ID        string
	Name      string
	LocalPath string
	Lang      string
}
