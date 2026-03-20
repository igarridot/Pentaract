package localfs

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Dominux/Pentaract/internal/domain"
)

type File struct {
	Path   string
	Name   string
	Size   int64
	Reader io.ReadCloser
}

type Source struct {
	root string
}

func New(root string) *Source {
	return &Source{root: root}
}

func (s *Source) ListDir(relPath string) ([]domain.FSElement, error) {
	root, err := s.configuredRoot()
	if err != nil {
		return nil, err
	}

	cleanPath, absPath, err := resolvePath(root, relPath, true)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, mapPathError(err)
	}
	if !info.IsDir() {
		return nil, domain.ErrBadRequest("local path is not a directory")
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, mapPathError(err)
	}

	items := make([]domain.FSElement, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if shouldSkipName(name) {
			continue
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			continue
		}

		switch {
		case entry.IsDir():
			items = append(items, domain.FSElement{
				Path:   joinPath(cleanPath, name),
				Name:   name,
				IsFile: false,
			})
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			items = append(items, domain.FSElement{
				Path:   joinPath(cleanPath, name),
				Name:   name,
				Size:   info.Size(),
				IsFile: true,
			})
		}
	}

	slices.SortFunc(items, func(a, b domain.FSElement) int {
		if a.IsFile != b.IsFile {
			if a.IsFile {
				return 1
			}
			return -1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	return items, nil
}

func (s *Source) ExpandSelection(paths []string) ([]domain.FSElement, error) {
	root, err := s.configuredRoot()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, domain.ErrBadRequest("paths are required")
	}

	seen := make(map[string]domain.FSElement)
	for _, selected := range paths {
		cleanPath, absPath, err := resolvePath(root, selected, false)
		if err != nil {
			return nil, err
		}

		info, err := os.Lstat(absPath)
		if err != nil {
			return nil, mapPathError(err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || shouldSkipName(path.Base(cleanPath)) {
			continue
		}

		switch {
		case info.IsDir():
			err = filepath.WalkDir(absPath, func(walkPath string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if walkPath == absPath {
					return nil
				}
				name := entry.Name()
				if shouldSkipName(name) {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.Type()&fs.ModeSymlink != 0 {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if entry.IsDir() || !entry.Type().IsRegular() {
					return nil
				}

				info, err := entry.Info()
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, walkPath)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				seen[rel] = domain.FSElement{
					Path:   rel,
					Name:   path.Base(rel),
					Size:   info.Size(),
					IsFile: true,
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		case info.Mode().IsRegular():
			seen[cleanPath] = domain.FSElement{
				Path:   cleanPath,
				Name:   path.Base(cleanPath),
				Size:   info.Size(),
				IsFile: true,
			}
		default:
			return nil, domain.ErrBadRequest("local selection must contain regular files or directories")
		}
	}

	files := make([]domain.FSElement, 0, len(seen))
	for _, item := range seen {
		files = append(files, item)
	}

	slices.SortFunc(files, func(a, b domain.FSElement) int {
		return strings.Compare(a.Path, b.Path)
	})

	return files, nil
}

func (s *Source) OpenFile(relPath string) (File, error) {
	root, err := s.configuredRoot()
	if err != nil {
		return File{}, err
	}

	cleanPath, absPath, err := resolvePath(root, relPath, false)
	if err != nil {
		return File{}, err
	}
	if shouldSkipName(path.Base(cleanPath)) {
		return File{}, domain.ErrBadRequest("source_path must reference a regular file")
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return File{}, mapPathError(err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return File{}, domain.ErrBadRequest("source_path must reference a regular file")
	}

	file, err := os.Open(absPath)
	if err != nil {
		return File{}, mapPathError(err)
	}

	return File{
		Path:   cleanPath,
		Name:   path.Base(cleanPath),
		Size:   info.Size(),
		Reader: file,
	}, nil
}

func (s *Source) configuredRoot() (string, error) {
	root := strings.TrimSpace(s.root)
	if root == "" {
		return "", domain.ErrBadRequest("local files source is not configured")
	}
	return filepath.Clean(root), nil
}

func resolvePath(root, relPath string, allowEmpty bool) (string, string, error) {
	cleanPath, err := cleanRelativePath(relPath, allowEmpty)
	if err != nil {
		return "", "", err
	}

	absPath := root
	if cleanPath != "" {
		absPath = filepath.Join(root, filepath.FromSlash(cleanPath))
	}
	absPath = filepath.Clean(absPath)

	if absPath != root && !strings.HasPrefix(absPath, root+string(os.PathSeparator)) {
		return "", "", domain.ErrBadRequest("invalid local path")
	}

	return cleanPath, absPath, nil
}

func cleanRelativePath(raw string, allowEmpty bool) (string, error) {
	clean := strings.ReplaceAll(raw, "\\", "/")
	clean = strings.Trim(clean, "/")
	if clean == "" {
		if allowEmpty {
			return "", nil
		}
		return "", domain.ErrBadRequest("local path is required")
	}

	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", domain.ErrBadRequest("invalid local path")
		}
	}

	return strings.Join(parts, "/"), nil
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

func shouldSkipName(name string) bool {
	return name == ".gitkeep"
}

func mapPathError(err error) error {
	if os.IsNotExist(err) {
		return domain.ErrNotFound("local path")
	}
	return err
}
