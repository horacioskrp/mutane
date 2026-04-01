package media

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Storage interface {
	Save(filename string, r io.Reader) (string, error)
	Delete(path string) error
	URL(path string) string
}

// LocalStorage saves files to disk under uploadDir.
type LocalStorage struct {
	uploadDir string
	baseURL   string
}

func NewLocalStorage() *LocalStorage {
	dir := os.Getenv("UPLOAD_DIR")
	if dir == "" {
		dir = "uploads"
	}
	baseURL := os.Getenv("UPLOAD_BASE_URL")
	if baseURL == "" {
		baseURL = "/uploads"
	}
	os.MkdirAll(dir, 0755)
	return &LocalStorage{uploadDir: dir, baseURL: baseURL}
}

func (s *LocalStorage) Save(filename string, r io.Reader) (string, error) {
	unique := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(filename))
	dst := filepath.Join(s.uploadDir, unique)

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return unique, nil
}

func (s *LocalStorage) Delete(path string) error {
	return os.Remove(filepath.Join(s.uploadDir, path))
}

func (s *LocalStorage) URL(path string) string {
	return s.baseURL + "/" + path
}
