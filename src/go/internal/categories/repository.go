package categories

import (
	"context"
	"database/sql"
)

type SQLRepository struct {
	database *sql.DB
}

func NewSQLRepository(database *sql.DB) *SQLRepository {
	return &SQLRepository{database: database}
}

func (repository *SQLRepository) List(ctx context.Context) ([]Category, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := repository.database.QueryContext(ctx, "SELECT id, name FROM CATEGORIES ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Category
	for rows.Next() {
		var category Category
		if err := rows.Scan(&category.ID, &category.Name); err != nil {
			return nil, err
		}
		result = append(result, category)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (repository *SQLRepository) Create(ctx context.Context, name string) (Category, error) {
	if repository.database == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "INSERT INTO CATEGORIES (name) VALUES (?)", name)
	if err != nil {
		return Category{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Category{}, err
	}
	return Category{ID: uint64(id), Name: name}, nil
}

func (repository *SQLRepository) Get(ctx context.Context, id uint64) (Category, error) {
	if repository.database == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	var result Category
	err := repository.database.QueryRowContext(ctx, "SELECT id, name FROM CATEGORIES WHERE id = ?", id).Scan(&result.ID, &result.Name)
	return result, err
}

func (repository *SQLRepository) Update(ctx context.Context, id uint64, name string) (Category, error) {
	if repository.database == nil {
		return Category{}, ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "UPDATE CATEGORIES SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return Category{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Category{}, err
	}
	if affected == 0 {
		return Category{}, sql.ErrNoRows
	}
	return Category{ID: id, Name: name}, nil
}

func (repository *SQLRepository) Delete(ctx context.Context, id uint64) error {
	if repository.database == nil {
		return ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "DELETE FROM CATEGORIES WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
