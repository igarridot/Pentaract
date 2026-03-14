package pathutil

import "strings"

// TrimTrailingSlash removes any trailing slash from a relative storage path.
func TrimTrailingSlash(path string) string {
	return strings.TrimRight(path, "/")
}

// Join normalizes path segments and joins them with a single slash.
func Join(base, name string) string {
	base = strings.Trim(base, "/")
	name = strings.Trim(name, "/")

	switch {
	case base == "":
		return name
	case name == "":
		return base
	default:
		return base + "/" + name
	}
}

// ArchiveName returns the filename to use when downloading a directory as zip.
func ArchiveName(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "files"
	}
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}
	return trimmed
}

// SplitDirAndFile separates a relative path into directory prefix and filename.
func SplitDirAndFile(path string) (dir, filename string) {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx+1], path[idx+1:]
	}
	return "", path
}

// SplitNameAndExtension separates a filename into stem and extension.
func SplitNameAndExtension(filename string) (name, ext string) {
	if idx := strings.LastIndex(filename, "."); idx > 0 {
		return filename[:idx], filename[idx:]
	}
	return filename, ""
}
