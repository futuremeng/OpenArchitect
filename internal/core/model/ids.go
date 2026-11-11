package model

import (
	"path"
	"path/filepath"
	"strings"
)

func SystemID(modulePath string) string { return "sys:" + modulePath }

func ContainerID(pkgPath string) string { return "ctn:" + pkgPath }

// ComponentID keys on the bare declaration name of a type
// (types.TypeName.Obj().Name()). Callers must drop generic instantiation
// parameters before calling (spec §6.2); this package does not strip them.
func ComponentID(pkgPath, typeName string) string {
	return "cmp:" + pkgPath + "." + typeName
}

// RepoIDFor keys on the cleaned local path. Absolutizing the path is the
// caller's obligation: this function only calls filepath.Clean, so a relative
// path and the same checkout's absolute path yield two different ids.
func RepoIDFor(localPath string) string { return "repo:" + filepath.Clean(localPath) }

// FileID keys on the module path plus the slash-relative path inside the
// module (PoC-13's fil: family). Callers pass the path already slash-
// separated and cleaned — the same string an Element's FilePath carries.
func FileID(modulePath, relPath string) string {
	return "fil:" + modulePath + "/" + filepath.ToSlash(relPath)
}

// PkgPathOf reverses the package part of a Container, Component or File id.
// For a File the answer is the import path of its directory: module path plus
// the relative dir, which is exactly what the go tool would call that package.
func PkgPathOf(id string) string {
	switch {
	case strings.HasPrefix(id, "ctn:"):
		return strings.TrimPrefix(id, "ctn:")
	case strings.HasPrefix(id, "cmp:"):
		body := strings.TrimPrefix(id, "cmp:")
		if i := strings.LastIndex(body, "."); i >= 0 {
			return body[:i]
		}
	case strings.HasPrefix(id, "fil:"):
		return path.Dir(strings.TrimPrefix(id, "fil:"))
	}
	return ""
}
