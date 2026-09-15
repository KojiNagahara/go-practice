package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Local struct {
	Root string
}

func NewLocal(root string) *Local {
	return &Local{Root: root}
}

func (local *Local) Put(ctx context.Context, object Object) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	name := SafeKey(object.Name)
	path := filepath.Join(local.Root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", err
	}
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := io.Copy(file, object.Reader); err != nil {
		return "", err
	}
	return name, nil
}

func (local *Local) ResolveURL(ctx context.Context, key string) (string, error) {
	if strings.Contains(key, "://") || strings.HasPrefix(key, "/") {
		return key, nil
	}
	return fmt.Sprintf("/uploads/%s", key), nil
}

func (local *Local) Delete(ctx context.Context, key string) error {
	if strings.Contains(key, "://") || strings.HasPrefix(key, "/") {
		return nil
	}
	return os.Remove(filepath.Join(local.Root, key))
}
