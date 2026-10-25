package facedetect

import (
	"errors"
	"os"
	"path/filepath"
)

// cacheDirFunc is the function used to resolve the model cache directory.
// It is a variable (not a plain function) so tests can override it with a
// temporary directory.
var cacheDirFunc = func() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "dataprotect", "face-models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// cacheDir returns the directory where face models are cached.
//
// It uses the OS-specific user cache directory:
//   - Linux: $XDG_CACHE_HOME or ~/.cache
//   - macOS: ~/Library/Caches
//   - Windows: %LOCALAPPDATA%
//
// Under that, models live in dataprotect/face-models/.
func cacheDir() (string, error) {
	return cacheDirFunc()
}

// modelPath returns the absolute path to a model file in the cache.
func modelPath(name string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// modelExists reports whether a model file is present in the cache.
func modelExists(name string) (bool, error) {
	path, err := modelPath(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// removeModel deletes a cached model file, if present. Missing files are
// silently ignored.
func removeModel(name string) error {
	path, err := modelPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// CacheStats reports information about the model cache directory.
type CacheStats struct {
	Dir       string
	Models    map[string]int64 // model name -> file size in bytes
	Missing   []string
	TotalSize int64
}

// Stats returns information about the current model cache.
func Stats() (CacheStats, error) {
	dir, err := cacheDir()
	if err != nil {
		return CacheStats{}, err
	}

	stats := CacheStats{
		Dir:     dir,
		Models:  make(map[string]int64, len(Models)),
		Missing: nil,
	}

	for _, m := range Models {
		path := filepath.Join(dir, m.Path)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				stats.Missing = append(stats.Missing, m.Name)
			}
			continue
		}
		stats.Models[m.Name] = info.Size()
		stats.TotalSize += info.Size()
	}

	return stats, nil
}
