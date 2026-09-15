package items

import (
	"context"
	"database/sql"
	"strings"
)

type SQLItemRepository struct {
	database *sql.DB
}

type SQLImageRepository struct {
	database *sql.DB
}

func NewSQLItemRepository(database *sql.DB) *SQLItemRepository {
	return &SQLItemRepository{database: database}
}

func NewSQLImageRepository(database *sql.DB) *SQLImageRepository {
	return &SQLImageRepository{database: database}
}

func NewSQLCategoryRepository(database *sql.DB) *SQLCategoryRepository {
	return &SQLCategoryRepository{database: database}
}

type SQLCategoryRepository struct {
	database *sql.DB
}

func (repository *SQLCategoryRepository) ListByItem(ctx context.Context, itemID uint64) ([]uint64, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := repository.database.QueryContext(ctx, "SELECT category_id FROM ITEM_CATEGORIES WHERE item_id = ? ORDER BY category_id", itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []uint64
	for rows.Next() {
		var categoryID uint64
		if err := rows.Scan(&categoryID); err != nil {
			return nil, err
		}
		result = append(result, categoryID)
	}
	return result, rows.Err()
}

func (repository *SQLCategoryRepository) ReplaceForItem(ctx context.Context, itemID uint64, categoryIDs []uint64) error {
	if repository.database == nil {
		return ErrDatabaseUnavailable
	}
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM ITEM_CATEGORIES WHERE item_id = ?", itemID); err != nil {
		_ = transaction.Rollback()
		return err
	}
	for _, categoryID := range categoryIDs {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO ITEM_CATEGORIES (item_id, category_id) VALUES (?, ?)", itemID, categoryID); err != nil {
			_ = transaction.Rollback()
			return err
		}
	}
	return transaction.Commit()
}

func (repository *SQLItemRepository) List(ctx context.Context) ([]Item, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}

	rows, err := repository.database.QueryContext(ctx, "SELECT id, name, item_description, price_minor_units FROM ITEMS ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Item
	for rows.Next() {
		var item Item
		var price int64
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &price); err != nil {
			return nil, err
		}
		item.Price = Money(price)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *SQLItemRepository) Search(ctx context.Context, filter SearchFilter) ([]Item, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	query := "SELECT DISTINCT i.id, i.name, i.item_description, i.price_minor_units FROM ITEMS i"
	args := make([]any, 0, 4+len(filter.CategoryIDs))
	conditions := make([]string, 0, 3)
	if len(filter.CategoryIDs) > 0 {
		placeholders := make([]string, len(filter.CategoryIDs))
		for index, categoryID := range filter.CategoryIDs {
			placeholders[index] = "?"
			args = append(args, categoryID)
		}
		query += " INNER JOIN ITEM_CATEGORIES ic ON ic.item_id = i.id"
		conditions = append(conditions, "ic.category_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if name := strings.TrimSpace(filter.Name); name != "" {
		conditions = append(conditions, "i.name LIKE ?")
		args = append(args, "%"+name+"%")
	}
	if filter.MinPrice != nil {
		conditions = append(conditions, "i.price_minor_units >= ?")
		args = append(args, filter.MinPrice.MinorUnits())
	}
	if filter.MaxPrice != nil {
		conditions = append(conditions, "i.price_minor_units <= ?")
		args = append(args, filter.MaxPrice.MinorUnits())
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY i.id"
	rows, err := repository.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Item, 0)
	for rows.Next() {
		var item Item
		var price int64
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &price); err != nil {
			return nil, err
		}
		item.Price = Money(price)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *SQLItemRepository) Create(ctx context.Context, name, description string, price Money) (Item, error) {
	if repository.database == nil {
		return Item{}, ErrDatabaseUnavailable
	}

	result, err := repository.database.ExecContext(ctx, "INSERT INTO ITEMS (name, item_description, price_minor_units) VALUES (?, ?, ?)", name, description, price.MinorUnits())
	if err != nil {
		return Item{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	return Item{ID: uint64(id), Name: name, Description: description, Price: price}, nil
}

func (repository *SQLItemRepository) CreateWithImages(ctx context.Context, name, description string, price Money, imageURLs []string) (Item, error) {
	if repository.database == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	transaction, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	result, err := transaction.ExecContext(ctx, "INSERT INTO ITEMS (name, item_description, price_minor_units) VALUES (?, ?, ?)", name, description, price.MinorUnits())
	if err != nil {
		_ = transaction.Rollback()
		return Item{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		_ = transaction.Rollback()
		return Item{}, err
	}
	for _, url := range imageURLs {
		if _, err := transaction.ExecContext(ctx, "INSERT INTO ITEM_IMAGES (item_id, object_key) VALUES (?, ?)", id, url); err != nil {
			_ = transaction.Rollback()
			return Item{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return Item{}, err
	}
	return Item{ID: uint64(id), Name: name, Description: description, Price: price}, nil
}

func (repository *SQLItemRepository) Get(ctx context.Context, id uint64) (Item, error) {
	if repository.database == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	var item Item
	var price int64
	err := repository.database.QueryRowContext(ctx, "SELECT id, name, item_description, price_minor_units FROM ITEMS WHERE id = ?", id).
		Scan(&item.ID, &item.Name, &item.Description, &price)
	item.Price = Money(price)
	return item, err
}

func (repository *SQLItemRepository) Update(ctx context.Context, id uint64, name, description string, price Money) (Item, error) {
	if repository.database == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "UPDATE ITEMS SET name = ?, item_description = ?, price_minor_units = ? WHERE id = ?", name, description, price.MinorUnits(), id)
	if err != nil {
		return Item{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Item{}, err
	}
	if affected == 0 {
		return repository.Get(ctx, id)
	}
	return Item{ID: id, Name: name, Description: description, Price: price}, nil
}

func (repository *SQLItemRepository) Delete(ctx context.Context, id uint64) error {
	if repository.database == nil {
		return ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "DELETE FROM ITEMS WHERE id = ?", id)
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

func (repository *SQLImageRepository) List(ctx context.Context) ([]Image, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := repository.database.QueryContext(ctx, "SELECT id, item_id, object_key FROM ITEM_IMAGES ORDER BY item_id, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Image
	for rows.Next() {
		var image Image
		if err := rows.Scan(&image.ID, &image.ItemID, &image.ObjectKey); err != nil {
			return nil, err
		}
		image.URL = image.ObjectKey
		result = append(result, image)
	}
	return result, rows.Err()
}

func (repository *SQLImageRepository) ListByItem(ctx context.Context, itemID uint64) ([]Image, error) {
	if repository.database == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := repository.database.QueryContext(ctx, "SELECT id, item_id, object_key FROM ITEM_IMAGES WHERE item_id = ? ORDER BY id", itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Image
	for rows.Next() {
		var image Image
		if err := rows.Scan(&image.ID, &image.ItemID, &image.ObjectKey); err != nil {
			return nil, err
		}
		image.URL = image.ObjectKey
		result = append(result, image)
	}
	return result, rows.Err()
}

func (repository *SQLImageRepository) Create(ctx context.Context, itemID uint64, url string) (Image, error) {
	if repository.database == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "INSERT INTO ITEM_IMAGES (item_id, object_key) VALUES (?, ?)", itemID, url)
	if err != nil {
		return Image{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Image{}, err
	}
	return Image{ID: uint64(id), ItemID: itemID, ObjectKey: url, URL: url}, nil
}

func (repository *SQLImageRepository) Get(ctx context.Context, id uint64) (Image, error) {
	if repository.database == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	var image Image
	err := repository.database.QueryRowContext(ctx, "SELECT id, item_id, object_key FROM ITEM_IMAGES WHERE id = ?", id).
		Scan(&image.ID, &image.ItemID, &image.ObjectKey)
	image.URL = image.ObjectKey
	return image, err
}

func (repository *SQLImageRepository) Update(ctx context.Context, id uint64, url string) (Image, error) {
	if repository.database == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "UPDATE ITEM_IMAGES SET object_key = ? WHERE id = ?", url, id)
	if err != nil {
		return Image{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Image{}, err
	}
	if affected == 0 {
		return Image{}, sql.ErrNoRows
	}
	image, err := repository.Get(ctx, id)
	if err != nil {
		return Image{}, err
	}
	return image, nil
}

func (repository *SQLImageRepository) Delete(ctx context.Context, id uint64) error {
	if repository.database == nil {
		return ErrDatabaseUnavailable
	}
	result, err := repository.database.ExecContext(ctx, "DELETE FROM ITEM_IMAGES WHERE id = ?", id)
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
