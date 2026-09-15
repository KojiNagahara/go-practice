package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type Object struct {
	Name        string
	ContentType string
	Reader      io.ReadCloser
}

type Storage interface {
	Put(context.Context, Object) (string, error)
}

type Resolver interface {
	ResolveURL(context.Context, string) (string, error)
}

type Deleter interface {
	Delete(context.Context, string) error
}

func SafeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return fmt.Sprintf("upload-%d", time.Now().UnixNano())
	}
	return name
}

func SafeKey(key string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(key), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	safe := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := SafeName(part); name != "" {
			safe = append(safe, name)
		}
	}
	return strings.Join(safe, "/")
}
