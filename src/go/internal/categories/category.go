package categories

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrDatabaseUnavailable = errors.New("database is not configured")
	ErrInvalidName         = errors.New("category name is required")
)

type Category struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

type Repository interface {
	List(context.Context) ([]Category, error)
	Create(context.Context, string) (Category, error)
	Get(context.Context, uint64) (Category, error)
	Update(context.Context, uint64, string) (Category, error)
	Delete(context.Context, uint64) error
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) List(ctx context.Context) ([]Category, error) {
	if service == nil || service.repository == nil {
		return nil, ErrDatabaseUnavailable
	}
	return service.repository.List(ctx)
}

func (service *Service) Create(ctx context.Context, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Category{}, ErrInvalidName
	}
	if service == nil || service.repository == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	return service.repository.Create(ctx, name)
}

func (service *Service) Get(ctx context.Context, id uint64) (Category, error) {
	if service == nil || service.repository == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	return service.repository.Get(ctx, id)
}

func (service *Service) Update(ctx context.Context, id uint64, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Category{}, ErrInvalidName
	}
	if service == nil || service.repository == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	return service.repository.Update(ctx, id, name)
}

func (service *Service) Delete(ctx context.Context, id uint64) error {
	if service == nil || service.repository == nil {
		return ErrDatabaseUnavailable
	}
	return service.repository.Delete(ctx, id)
}
