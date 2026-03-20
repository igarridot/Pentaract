package localfs

import (
	"errors"
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

type rootedReadCloser struct {
	file *os.File
	root *os.Root
}

func New(root string) *Source {
	return &Source{root: root}
}

func (r *rootedReadCloser) Read(p []byte) (int, error) {
	return r.file.Read(p)
}

func (r *rootedReadCloser) Close() error {
	return errors.Join(r.file.Close(), r.root.Close())
}

func (s *Source) ListDir(relPath string) ([]domain.FSElement, error) {
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()

	cleanPath, rootPath, err := resolvePath(relPath, true)
	if err != nil {
		return nil, err
	}

	dir, err := root.Open(rootPath)
	if err != nil {
		return nil, mapPathError(err)
	}
	defer dir.Close()

	info, err := dir.Stat()
	if err != nil {
		return nil, mapPathError(err)
	}
	if !info.IsDir() {
		return nil, domain.ErrBadRequest("local path is not a directory")
	}

	entries, err := dir.ReadDir(-1)
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
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if len(paths) == 0 {
		return nil, domain.ErrBadRequest("paths are required")
	}

	seen := make(map[string]domain.FSElement)
	for _, selected := range paths {
		cleanPath, rootPath, err := resolvePath(selected, false)
		if err != nil {
			return nil, err
		}

		info, err := root.Lstat(rootPath)
		if err != nil {
			return nil, mapPathError(err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || shouldSkipName(path.Base(cleanPath)) {
			continue
		}

		switch {
		case info.IsDir():
			dirRoot, err := root.OpenRoot(rootPath)
			if err != nil {
				return nil, mapPathError(err)
			}
			err = fs.WalkDir(dirRoot.FS(), ".", func(walkPath string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if walkPath == "." {
					return nil
				}
				name := entry.Name()
				if shouldSkipName(name) {
					if entry.IsDir() {
						return fs.SkipDir
					}
					return nil
				}
				if entry.Type()&fs.ModeSymlink != 0 {
					if entry.IsDir() {
						return fs.SkipDir
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
				rel := joinPath(cleanPath, walkPath)
				seen[rel] = domain.FSElement{
					Path:   rel,
					Name:   path.Base(rel),
					Size:   info.Size(),
					IsFile: true,
				}
				return nil
			})
			closeErr := dirRoot.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return nil, mapPathError(err)
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
	root, err := s.openRoot()
	if err != nil {
		return File{}, err
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = root.Close()
		}
	}()

	cleanPath, rootPath, err := resolvePath(relPath, false)
	if err != nil {
		return File{}, err
	}
	if shouldSkipName(path.Base(cleanPath)) {
		return File{}, domain.ErrBadRequest("source_path must reference a regular file")
	}

	info, err := root.Lstat(rootPath)
	if err != nil {
		return File{}, mapPathError(err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return File{}, domain.ErrBadRequest("source_path must reference a regular file")
	}

	file, err := root.Open(rootPath)
	if err != nil {
		return File{}, mapPathError(err)
	}
	closeRoot = false

	return File{
		Path:   cleanPath,
		Name:   path.Base(cleanPath),
		Size:   info.Size(),
		Reader: &rootedReadCloser{file: file, root: root},
	}, nil
}

func (s *Source) configuredRoot() (string, error) {
	root := strings.TrimSpace(s.root)
	if root == "" {
		return "", domain.ErrBadRequest("local files source is not configured")
	}
	return filepath.Clean(root), nil
}

func (s *Source) openRoot() (*os.Root, error) {
	root, err := s.configuredRoot()
	if err != nil {
		return nil, err
	}

	openedRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, mapPathError(err)
	}

	return openedRoot, nil
}

func resolvePath(relPath string, allowEmpty bool) (string, string, error) {
	cleanPath, err := cleanRelativePath(relPath, allowEmpty)
	if err != nil {
		return "", "", err
	}

	rootPath := cleanPath
	if rootPath == "" {
		rootPath = "."
	}

	return cleanPath, rootPath, nil
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
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil && pathErr.Err.Error() == "path escapes from parent" {
		return domain.ErrBadRequest("invalid local path")
	}
	return err
}
