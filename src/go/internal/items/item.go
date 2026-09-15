package items

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go-practice/internal/storage"
)

var (
	ErrDatabaseUnavailable = errors.New("database is not configured")
	ErrInvalidName         = errors.New("item name is required")
	ErrInvalidDescription  = errors.New("item description is required")
	ErrInvalidPrice        = errors.New("item price must be zero or greater")
	ErrInvalidImageURL     = errors.New("image URL is required")
	ErrInvalidMoney        = errors.New("item price must have at most two decimal places")
	ErrImageNotOwned       = errors.New("image does not belong to item")
)

// Money stores a monetary amount as integer minor units (cents).
type Money int64

func NewMoney(minorUnits int64) (Money, error) {
	if minorUnits < 0 {
		return 0, ErrInvalidPrice
	}
	return Money(minorUnits), nil
}

func ParseMoney(value string) (Money, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, ErrInvalidMoney
	}
	if strings.HasPrefix(value, "-") {
		return 0, ErrInvalidPrice
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalidMoney
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, ErrInvalidMoney
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > 2 {
			return 0, ErrInvalidMoney
		}
	}
	fraction = fraction + strings.Repeat("0", 2-len(fraction))
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, ErrInvalidMoney
	}
	if whole > (int64(^uint64(0)>>1)-cents)/100 {
		return 0, ErrInvalidMoney
	}
	return Money(whole*100 + cents), nil
}

func (money Money) MinorUnits() int64 {
	return int64(money)
}

func (money Money) String() string {
	return fmt.Sprintf("%d.%02d", int64(money)/100, int64(money)%100)
}

func (money Money) MarshalJSON() ([]byte, error) {
	if money < 0 {
		return nil, ErrInvalidPrice
	}
	return []byte(money.String()), nil
}

func (money *Money) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	var value json.Number
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		parsed, err := ParseMoney(text)
		if err != nil {
			return err
		}
		*money = parsed
		return nil
	}
	value = json.Number(strings.TrimSpace(string(data)))
	parsed, err := ParseMoney(value.String())
	if err != nil {
		return err
	}
	*money = parsed
	return nil
}

type Item struct {
	ID          uint64   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       Money    `json:"price"`
	Images      []Image  `json:"-"`
	CategoryIDs []uint64 `json:"category_ids"`
}

type Image struct {
	ID        uint64 `json:"id"`
	ItemID    uint64 `json:"item_id"`
	ObjectKey string `json:"-"`
	URL       string `json:"image_url"`
}

type ItemWithImages struct {
	Item
	Images []Image `json:"images"`
}

func (item Item) HasCategory(categoryID uint64) bool {
	for _, assignedID := range item.CategoryIDs {
		if assignedID == categoryID {
			return true
		}
	}
	return false
}

type ItemRepository interface {
	List(context.Context) ([]Item, error)
	Create(context.Context, string, string, Money) (Item, error)
	CreateWithImages(context.Context, string, string, Money, []string) (Item, error)
	Get(context.Context, uint64) (Item, error)
	Update(context.Context, uint64, string, string, Money) (Item, error)
	Delete(context.Context, uint64) error
}

type SearchFilter struct {
	Name        string
	CategoryIDs []uint64
	MinPrice    *Money
	MaxPrice    *Money
}

type CategoryRepository interface {
	ListByItem(context.Context, uint64) ([]uint64, error)
	ReplaceForItem(context.Context, uint64, []uint64) error
}

type ImageRepository interface {
	List(context.Context) ([]Image, error)
	ListByItem(context.Context, uint64) ([]Image, error)
	Create(context.Context, uint64, string) (Image, error)
	Get(context.Context, uint64) (Image, error)
	Update(context.Context, uint64, string) (Image, error)
	Delete(context.Context, uint64) error
}

type Service struct {
	items    ItemRepository
	images   ImageRepository
	storage  storage.Storage
	category CategoryRepository
}

func NewService(itemRepository ItemRepository, imageRepository ImageRepository, objectStorage storage.Storage, categoryRepositories ...CategoryRepository) *Service {
	var categoryRepository CategoryRepository
	if len(categoryRepositories) > 0 {
		categoryRepository = categoryRepositories[0]
	}
	return &Service{items: itemRepository, images: imageRepository, storage: objectStorage, category: categoryRepository}
}

func (service *Service) CreateWithUploads(ctx context.Context, name, description string, price Money, uploads []storage.Object) (Item, error) {
	return service.CreateWithUploadsAndCategories(ctx, name, description, price, nil, uploads)
}

func (service *Service) CreateWithUploadsAndCategories(ctx context.Context, name, description string, price Money, categoryIDs []uint64, uploads []storage.Object) (Item, error) {
	if service == nil || service.storage == nil || service.images == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	item, err := service.Create(ctx, name, description, price)
	if err != nil {
		return Item{}, err
	}
	if err := service.setCategories(ctx, item.ID, categoryIDs); err != nil {
		_ = service.items.Delete(ctx, item.ID)
		return Item{}, err
	}
	item.CategoryIDs = append([]uint64(nil), categoryIDs...)
	keys := make([]string, 0, len(uploads))
	images := make([]Image, 0, len(uploads))
	for _, upload := range uploads {
		if upload.Reader != nil {
			defer upload.Reader.Close()
		}
		upload.Name = storageObjectName(item.ID, upload.Name)
		key, err := service.storage.Put(ctx, upload)
		if err != nil {
			service.cleanupUploaded(ctx, keys, images)
			_ = service.items.Delete(ctx, item.ID)
			return Item{}, err
		}
		image, err := service.images.Create(ctx, item.ID, key)
		if err != nil {
			service.cleanupUploaded(ctx, append(keys, key), images)
			_ = service.items.Delete(ctx, item.ID)
			return Item{}, err
		}
		if err := item.AddImage(image); err != nil {
			service.cleanupUploaded(ctx, append(keys, key), append(images, image))
			_ = service.items.Delete(ctx, item.ID)
			return Item{}, err
		}
		keys = append(keys, key)
		images = append(images, image)
	}
	return item, nil
}

func storageObjectName(itemID uint64, name string) string {
	return strings.Join([]string{"items", fmt.Sprint(itemID), storage.SafeName(name)}, "/")
}

func (service *Service) CreateImageWithUpload(ctx context.Context, itemID uint64, upload storage.Object) (Image, error) {
	if service == nil || service.storage == nil || service.images == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	if upload.Reader != nil {
		defer upload.Reader.Close()
	}
	if _, err := service.Get(ctx, itemID); err != nil {
		return Image{}, err
	}
	upload.Name = storageObjectName(itemID, upload.Name)
	key, err := service.storage.Put(ctx, upload)
	if err != nil {
		return Image{}, err
	}
	image, err := service.images.Create(ctx, itemID, key)
	if err != nil {
		service.deleteObject(ctx, key)
		return Image{}, err
	}
	return service.resolveImage(ctx, image)
}

func (service *Service) List(ctx context.Context) ([]ItemWithImages, error) {
	if service == nil || service.items == nil || service.images == nil {
		return nil, ErrDatabaseUnavailable
	}

	items, err := service.items.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ItemWithImages, 0, len(items))
	for _, item := range items {
		images, err := service.images.ListByItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		images, err = service.resolveImages(ctx, images)
		if err != nil {
			return nil, err
		}
		item.Images = images
		item.CategoryIDs, err = service.categoriesForItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, ItemWithImages{Item: item, Images: images})
	}
	return result, nil
}

func (service *Service) Search(ctx context.Context, filter SearchFilter) ([]ItemWithImages, error) {
	if service == nil || service.items == nil || service.images == nil {
		return nil, ErrDatabaseUnavailable
	}
	repository, ok := service.items.(interface {
		Search(context.Context, SearchFilter) ([]Item, error)
	})
	if !ok {
		return nil, ErrDatabaseUnavailable
	}
	items, err := repository.Search(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := make([]ItemWithImages, 0, len(items))
	for _, item := range items {
		images, err := service.images.ListByItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		images, err = service.resolveImages(ctx, images)
		if err != nil {
			return nil, err
		}
		item.Images = images
		item.CategoryIDs, err = service.categoriesForItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, ItemWithImages{Item: item, Images: images})
	}
	return result, nil
}

func (service *Service) Create(ctx context.Context, name, description string, price Money) (Item, error) {
	return service.CreateWithImages(ctx, name, description, price, nil)
}

func (service *Service) CreateWithCategories(ctx context.Context, name, description string, price Money, categoryIDs []uint64) (Item, error) {
	item, err := service.Create(ctx, name, description, price)
	if err != nil {
		return Item{}, err
	}
	if err := service.setCategories(ctx, item.ID, categoryIDs); err != nil {
		_ = service.items.Delete(ctx, item.ID)
		return Item{}, err
	}
	item.CategoryIDs = append([]uint64(nil), categoryIDs...)
	return item, nil
}

func (service *Service) CreateWithImages(ctx context.Context, name, description string, price Money, imageURLs []string) (Item, error) {
	name, description, err := validateItem(name, description, price)
	if err != nil {
		return Item{}, err
	}
	if service == nil || service.items == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	for _, url := range imageURLs {
		if strings.TrimSpace(url) == "" {
			return Item{}, ErrInvalidImageURL
		}
	}
	if len(imageURLs) > 0 {
		return service.items.CreateWithImages(ctx, name, description, price, imageURLs)
	}
	return service.items.Create(ctx, name, description, price)
}

func (service *Service) Get(ctx context.Context, id uint64) (Item, error) {
	if service == nil || service.items == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	item, err := service.items.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	item.CategoryIDs, err = service.categoriesForItem(ctx, id)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func (service *Service) GetWithImages(ctx context.Context, id uint64) (ItemWithImages, error) {
	if service == nil || service.images == nil {
		return ItemWithImages{}, ErrDatabaseUnavailable
	}
	item, err := service.Get(ctx, id)
	if err != nil {
		return ItemWithImages{}, err
	}
	images, err := service.images.ListByItem(ctx, id)
	if err != nil {
		return ItemWithImages{}, err
	}
	images, err = service.resolveImages(ctx, images)
	if err != nil {
		return ItemWithImages{}, err
	}
	item.Images = images
	item.CategoryIDs, err = service.categoriesForItem(ctx, id)
	if err != nil {
		return ItemWithImages{}, err
	}
	return ItemWithImages{Item: item, Images: images}, nil
}

func (service *Service) UpdateItemCategories(ctx context.Context, id uint64, categoryIDs []uint64) error {
	if err := service.setCategories(ctx, id, categoryIDs); err != nil {
		return err
	}
	return nil
}

func (service *Service) setCategories(ctx context.Context, itemID uint64, categoryIDs []uint64) error {
	if service.category == nil {
		return nil
	}
	return service.category.ReplaceForItem(ctx, itemID, uniqueIDs(categoryIDs))
}

func (service *Service) categoriesForItem(ctx context.Context, itemID uint64) ([]uint64, error) {
	if service.category == nil {
		return nil, nil
	}
	return service.category.ListByItem(ctx, itemID)
}

func uniqueIDs(ids []uint64) []uint64 {
	result := make([]uint64, 0, len(ids))
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// UpdateItem is the aggregate use case for editing item fields and images together.
func (service *Service) UpdateItem(ctx context.Context, id uint64, name, description string, price Money, categoryIDs, deleteImageIDs []uint64, uploads []storage.Object) (ItemWithImages, error) {
	item, err := service.GetWithImages(ctx, id)
	if err != nil {
		return ItemWithImages{}, err
	}
	if err := service.setCategories(ctx, id, categoryIDs); err != nil {
		return ItemWithImages{}, err
	}
	name, description, err = validateItem(name, description, price)
	if err != nil {
		return ItemWithImages{}, err
	}
	for _, imageID := range deleteImageIDs {
		if _, err := item.RemoveImage(imageID); err != nil {
			return ItemWithImages{}, err
		}
	}
	created := make([]Image, 0, len(uploads))
	keys := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		image, key, uploadErr := service.putImage(ctx, id, upload)
		if uploadErr != nil {
			service.cleanupUploaded(ctx, keys, created)
			return ItemWithImages{}, uploadErr
		}
		if err := item.AddImage(image); err != nil {
			service.cleanupUploaded(ctx, append(keys, key), append(created, image))
			return ItemWithImages{}, err
		}
		created = append(created, image)
		keys = append(keys, key)
	}
	updated, err := service.items.Update(ctx, id, name, description, price)
	if err != nil {
		service.cleanupUploaded(ctx, keys, created)
		return ItemWithImages{}, err
	}
	for _, imageID := range deleteImageIDs {
		if err := service.DeleteImage(ctx, imageID); err != nil {
			_, _ = service.items.Update(ctx, id, item.Name, item.Description, item.Price)
			service.cleanupUploaded(ctx, keys, created)
			return ItemWithImages{}, err
		}
	}
	updated.Images = item.Images
	updated.CategoryIDs = item.CategoryIDs
	updated.Images, err = service.resolveImages(ctx, updated.Images)
	if err != nil {
		return ItemWithImages{}, err
	}
	return ItemWithImages{Item: updated, Images: updated.Images}, nil
}

func (service *Service) UpdateWithImages(ctx context.Context, id uint64, name, description string, price Money, deleteImageIDs []uint64, uploads []storage.Object) (ItemWithImages, error) {
	return service.UpdateItem(ctx, id, name, description, price, nil, deleteImageIDs, uploads)
}

func (service *Service) Update(ctx context.Context, id uint64, name, description string, price Money) (Item, error) {
	name, description, err := validateItem(name, description, price)
	if err != nil {
		return Item{}, err
	}
	if service == nil || service.items == nil {
		return Item{}, ErrDatabaseUnavailable
	}
	return service.items.Update(ctx, id, name, description, price)
}

func (service *Service) Delete(ctx context.Context, id uint64) error {
	if service == nil || service.items == nil {
		return ErrDatabaseUnavailable
	}
	var images []Image
	if service.images != nil {
		var err error
		images, err = service.images.ListByItem(ctx, id)
		if err != nil {
			return err
		}
	}
	if err := service.items.Delete(ctx, id); err != nil {
		return err
	}
	for _, image := range images {
		service.deleteObject(ctx, image.ObjectKey)
	}
	return nil
}

func (service *Service) ListImages(ctx context.Context) ([]Image, error) {
	if service == nil || service.images == nil {
		return nil, ErrDatabaseUnavailable
	}
	images, err := service.images.List(ctx)
	if err != nil {
		return nil, err
	}
	return service.resolveImages(ctx, images)
}

func (service *Service) CreateImage(ctx context.Context, itemID uint64, url string) (Image, error) {
	url = strings.TrimSpace(url)
	if itemID == 0 || url == "" {
		return Image{}, ErrInvalidImageURL
	}
	if service == nil || service.images == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	if _, err := service.Get(ctx, itemID); err != nil {
		return Image{}, err
	}
	image, err := service.images.Create(ctx, itemID, url)
	if err != nil {
		return Image{}, err
	}
	return service.resolveImage(ctx, image)
}

func (service *Service) GetImage(ctx context.Context, id uint64) (Image, error) {
	if service == nil || service.images == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	image, err := service.images.Get(ctx, id)
	if err != nil {
		return Image{}, err
	}
	return service.resolveImage(ctx, image)
}

func (service *Service) UpdateImage(ctx context.Context, id uint64, url string) (Image, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return Image{}, ErrInvalidImageURL
	}
	if service == nil || service.images == nil {
		return Image{}, ErrDatabaseUnavailable
	}
	existing, err := service.images.Get(ctx, id)
	if err != nil {
		return Image{}, err
	}
	if _, err := service.Get(ctx, existing.ItemID); err != nil {
		return Image{}, err
	}
	image, err := service.images.Update(ctx, id, url)
	if err != nil {
		return Image{}, err
	}
	if existing.ObjectKey != url {
		service.deleteObject(ctx, existing.ObjectKey)
	}
	return service.resolveImage(ctx, image)
}

func (service *Service) DeleteImage(ctx context.Context, id uint64) error {
	if service == nil || service.images == nil {
		return ErrDatabaseUnavailable
	}
	image, err := service.images.Get(ctx, id)
	if err != nil {
		return err
	}
	if _, err := service.Get(ctx, image.ItemID); err != nil {
		return err
	}
	if err := service.images.Delete(ctx, id); err != nil {
		return err
	}
	service.deleteObject(ctx, image.ObjectKey)
	return nil
}

func validateItem(name, description string, price Money) (string, string, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return "", "", ErrInvalidName
	}
	if description == "" {
		return "", "", ErrInvalidDescription
	}
	if price < 0 {
		return "", "", ErrInvalidPrice
	}
	return name, description, nil
}

func (item *Item) AddImage(image Image) error {
	if item == nil || item.ID == 0 || image.ItemID != item.ID {
		return ErrImageNotOwned
	}
	item.Images = append(item.Images, image)
	return nil
}

func (item *Item) RemoveImage(id uint64) (Image, error) {
	for index, image := range item.Images {
		if image.ID == id {
			item.Images = append(item.Images[:index], item.Images[index+1:]...)
			return image, nil
		}
	}
	return Image{}, sql.ErrNoRows
}

func (service *Service) putImage(ctx context.Context, itemID uint64, upload storage.Object) (Image, string, error) {
	if upload.Reader != nil {
		defer upload.Reader.Close()
	}
	upload.Name = storageObjectName(itemID, upload.Name)
	key, err := service.storage.Put(ctx, upload)
	if err != nil {
		return Image{}, "", err
	}
	image, err := service.images.Create(ctx, itemID, key)
	if err != nil {
		service.deleteObject(ctx, key)
		return Image{}, "", err
	}
	return image, key, nil
}

func (service *Service) cleanupUploaded(ctx context.Context, keys []string, images []Image) {
	for _, image := range images {
		_ = service.images.Delete(ctx, image.ID)
	}
	for _, key := range keys {
		service.deleteObject(ctx, key)
	}
}

func (service *Service) deleteObject(ctx context.Context, key string) {
	if deleter, ok := service.storage.(storage.Deleter); ok && key != "" && !strings.Contains(key, "://") {
		_ = deleter.Delete(ctx, key)
	}
}

func (service *Service) resolveImage(ctx context.Context, image Image) (Image, error) {
	if image.ObjectKey == "" {
		image.ObjectKey = image.URL
	}
	image.URL = image.ObjectKey
	if resolver, ok := service.storage.(storage.Resolver); ok &&
		!strings.Contains(image.ObjectKey, "://") && !strings.HasPrefix(image.ObjectKey, "/") {
		var err error
		image.URL, err = resolver.ResolveURL(ctx, image.ObjectKey)
		if err != nil {
			return Image{}, err
		}
	}
	return image, nil
}

func (service *Service) resolveImages(ctx context.Context, images []Image) ([]Image, error) {
	result := make([]Image, 0, len(images))
	for _, image := range images {
		resolved, err := service.resolveImage(ctx, image)
		if err != nil {
			return nil, err
		}
		result = append(result, resolved)
	}
	return result, nil
}
