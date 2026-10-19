package model

import (
	"path/filepath"
	"strings"
)

func SystemID(modulePath string) string { return "sys:" + modulePath }

func ContainerID(pkgPath string) string { return "ctn:" + pkgPath }

func ComponentID(pkgPath, typeName string) string {
	return "cmp:" + pkgPath + "." + typeName
}

// RepoIDFor keys on the absolute local path; the same checkout is the same row.
func RepoIDFor(localPath string) string { return "repo:" + filepath.Clean(localPath) }

// PkgPathOf reverses the package part of a Container or Component id.
func PkgPathOf(id string) string {
	switch {
	case strings.HasPrefix(id, "ctn:"):
		return strings.TrimPrefix(id, "ctn:")
	case strings.HasPrefix(id, "cmp:"):
		body := strings.TrimPrefix(id, "cmp:")
		if i := strings.LastIndex(body, "."); i >= 0 {
			return body[:i]
		}
	}
	return ""
}
