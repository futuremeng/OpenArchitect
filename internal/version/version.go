// Package version is the single release identity shared by both faces and
// the self-updater. The release workflow refuses to publish when the git tag
// does not equal "v"+Version, so this constant and GitHub Releases can never
// drift apart.
package version

// Version is the release identity of both faces (1.0 plan T10).
const Version = "1.1.0"
