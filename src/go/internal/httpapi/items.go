package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go-practice/internal/categories"
	"go-practice/internal/items"
	"go-practice/internal/storage"
	"go-practice/view"
)

type itemHandler struct {
	service         *items.Service
	itemsTemplate   *template.Template
	imagesTemplate  *template.Template
	editTemplate    *template.Template
	shopTemplate    *template.Template
	categoryService *categories.Service
}

func newItemHandler(service *items.Service, categoryService *categories.Service) *itemHandler {
	return &itemHandler{
		service:        service,
		itemsTemplate:  template.Must(template.ParseFS(view.Files, "items.html")),
		imagesTemplate: template.Must(template.ParseFS(view.Files, "item_images.html")),
		editTemplate:   template.Must(template.ParseFS(view.Files, "edit_item.html")),
		shopTemplate: template.Must(template.New("shop.html").Funcs(template.FuncMap{
			"hasID": func(ids []uint64, id uint64) bool {
				for _, value := range ids {
					if value == id {
						return true
					}
				}
				return false
			},
		}).ParseFS(view.Files, "shop.html")),
		categoryService: categoryService,
	}
}

func (handler *itemHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/items", handler.itemsPage)
	mux.HandleFunc("/items/", handler.itemsPage)
	mux.HandleFunc("/shop", handler.shopPage)
	mux.HandleFunc("/shop/", handler.shopPage)
	mux.HandleFunc("/item-images", handler.imagesPage)
	mux.HandleFunc("/item-images/", handler.imagesPage)
	mux.HandleFunc("/api/items", handler.itemCollection)
	mux.HandleFunc("/api/items/", handler.itemResource)
	mux.HandleFunc("/api/item-images", handler.imageCollection)
	mux.HandleFunc("/api/item-images/", handler.imageResource)
}

type shopItem struct {
	items.ItemWithImages
	CategoryNames []string
}

type shopPageModel struct {
	Items       []shopItem
	Categories  []categories.Category
	Name        string
	CategoryIDs []uint64
	MinPrice    string
	MaxPrice    string
	Query       string
	pageData
}

func (handler *itemHandler) shopPage(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/shop" && request.URL.Path != "/shop/" {
		http.NotFound(response, request)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := request.URL.Query()
	filter := items.SearchFilter{Name: query.Get("name"), CategoryIDs: parseIDs(query["category_id"])}
	minPrice, err := parseOptionalMoney(query.Get("min_price"))
	if err != nil {
		http.Error(response, "最低価格が不正です", http.StatusBadRequest)
		return
	}
	maxPrice, err := parseOptionalMoney(query.Get("max_price"))
	if err != nil {
		http.Error(response, "最高価格が不正です", http.StatusBadRequest)
		return
	}
	filter.MinPrice, filter.MaxPrice = minPrice, maxPrice
	result, err := handler.service.Search(request.Context(), filter)
	if err != nil {
		http.Error(response, "商品検索に失敗しました", itemStatus(err))
		return
	}
	availableCategories, err := handler.availableCategories(request.Context())
	if err != nil {
		http.Error(response, "カテゴリ読み込みに失敗しました", http.StatusInternalServerError)
		return
	}
	categoryNames := make(map[uint64]string, len(availableCategories))
	for _, category := range availableCategories {
		categoryNames[category.ID] = category.Name
	}
	shopItems := make([]shopItem, 0, len(result))
	for _, item := range result {
		names := make([]string, 0, len(item.CategoryIDs))
		for _, categoryID := range item.CategoryIDs {
			if name, ok := categoryNames[categoryID]; ok {
				names = append(names, name)
			}
		}
		shopItems = append(shopItems, shopItem{ItemWithImages: item, CategoryNames: names})
	}
	page, perPage, start, end := paginate(request, len(shopItems))
	model := shopPageModel{
		Items: shopItems[start:end], Categories: availableCategories,
		Name: query.Get("name"), CategoryIDs: filter.CategoryIDs,
		MinPrice: query.Get("min_price"), MaxPrice: query.Get("max_price"),
		Query:    shopQuery(query),
		pageData: newPageData(nil, page, perPage, len(shopItems)),
	}

	if err := handler.shopTemplate.Execute(response, model); err != nil {
		http.Error(response, "商品一覧画面の表示に失敗しました", http.StatusInternalServerError)
	}

}

func shopQuery(query url.Values) string {
	values := make(url.Values, len(query))
	for key, entries := range query {
		if key == "page" || key == "per_page" {
			continue
		}
		values[key] = append([]string(nil), entries...)
	}
	return values.Encode()
}

func parseOptionalMoney(value string) (*items.Money, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	money, err := items.ParseMoney(value)
	if err != nil {
		return nil, err
	}
	return &money, nil
}

func itemStylesheetHandler(response http.ResponseWriter, request *http.Request) {
	stylesheet, err := view.Files.ReadFile("items.css")
	if err != nil {
		http.Error(response, "stylesheet unavailable", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/css; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(stylesheet)
}

func (handler *itemHandler) editItemPage(response http.ResponseWriter, request *http.Request) {
	idText := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/items/"), "/edit")
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil || id == 0 {
		http.NotFound(response, request)
		return
	}
	if request.Method == http.MethodPost {
		input, err := parseItemForm(request)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		uploads, err := parseUploads(request)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		deleteIDs := make([]uint64, 0, len(request.Form["delete_image_id"]))
		for _, value := range request.Form["delete_image_id"] {
			imageID, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr != nil || imageID == 0 {
				http.Error(response, "invalid image id", http.StatusBadRequest)
				return
			}
			deleteIDs = append(deleteIDs, imageID)
		}
		categoryIDs := parseIDs(request.Form["category_id"])
		if _, err := handler.service.UpdateItem(request.Context(), id, input.Name, input.Description, input.Price, categoryIDs, deleteIDs, uploads); err != nil {
			http.Error(response, itemPageError(err, "item update failed"), itemStatus(err))
			return
		}
		http.Redirect(response, request, "/items", http.StatusSeeOther)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := handler.service.GetWithImages(request.Context(), id)
	if err != nil {
		http.Error(response, "item loading failed", itemStatus(err))
		return
	}
	availableCategories, err := handler.availableCategories(request.Context())
	if err != nil {
		http.Error(response, "category loading failed", http.StatusInternalServerError)
		return
	}
	if err := handler.editTemplate.Execute(response, struct {
		items.ItemWithImages
		Categories []categories.Category
	}{result, availableCategories}); err != nil {
		http.Error(response, "item edit page rendering failed", http.StatusInternalServerError)
	}
}

func (handler *itemHandler) itemsPage(response http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/items/") && strings.HasSuffix(request.URL.Path, "/edit") {
		handler.editItemPage(response, request)
		return
	}
	if request.URL.Path != "/items" && request.URL.Path != "/items/" {
		http.NotFound(response, request)
		return
	}
	if request.Method == http.MethodPost {
		input, err := parseItemForm(request)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		uploads, err := parseUploads(request)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		categoryIDs := parseIDs(request.Form["category_id"])
		if len(uploads) > 0 {
			if _, err := handler.service.CreateWithUploadsAndCategories(request.Context(), input.Name, input.Description, input.Price, categoryIDs, uploads); err != nil {
				http.Error(response, itemPageError(err, "item creation failed"), itemStatus(err))
				return
			}
		} else if _, err := handler.service.CreateWithCategories(request.Context(), input.Name, input.Description, input.Price, categoryIDs); err != nil {
			http.Error(response, itemPageError(err, "item creation failed"), itemStatus(err))
			return
		}
		http.Redirect(response, request, "/items", http.StatusSeeOther)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := handler.service.List(request.Context())
	if err != nil {
		http.Error(response, "item loading failed", itemStatus(err))
		return
	}
	page, perPage, start, end := paginate(request, len(result))
	availableCategories, err := handler.availableCategories(request.Context())
	if err != nil {
		http.Error(response, "category loading failed", http.StatusInternalServerError)
		return
	}
	pageModel := newPageData(result[start:end], page, perPage, len(result))
	if err := handler.itemsTemplate.Execute(response, struct {
		pageData
		Categories []categories.Category
	}{pageModel, availableCategories}); err != nil {
		http.Error(response, "item page rendering failed", http.StatusInternalServerError)
	}

}

func (handler *itemHandler) availableCategories(ctx context.Context) ([]categories.Category, error) {
	if handler.categoryService == nil {
		return nil, nil
	}
	return handler.categoryService.List(ctx)
}

func parseIDs(values []string) []uint64 {
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(value, 10, 64)
		if err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func (handler *itemHandler) imagesPage(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/item-images" && request.URL.Path != "/item-images/" {
		http.NotFound(response, request)
		return
	}
	if request.Method == http.MethodPost {
		itemID, err := strconv.ParseUint(strings.TrimSpace(request.FormValue("item_id")), 10, 64)
		if err != nil || itemID == 0 {
			http.Error(response, "valid item_id is required", http.StatusBadRequest)
			return
		}
		uploads, err := parseUploads(request)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		if len(uploads) > 0 {
			for _, upload := range uploads {
				if _, err := handler.service.CreateImageWithUpload(request.Context(), itemID, upload); err != nil {
					http.Error(response, itemPageError(err, "image creation failed"), itemStatus(err))
					return
				}
			}
			http.Redirect(response, request, "/item-images", http.StatusSeeOther)
			return
		}
		if _, err := handler.service.CreateImage(request.Context(), itemID, request.FormValue("image_url")); err != nil {
			http.Error(response, itemPageError(err, "image creation failed"), itemStatus(err))
			return
		}
		http.Redirect(response, request, "/item-images", http.StatusSeeOther)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := handler.service.ListImages(request.Context())
	if err != nil {
		http.Error(response, "image loading failed", itemStatus(err))
		return
	}
	page, perPage, start, end := paginate(request, len(result))
	if err := handler.imagesTemplate.Execute(response, newPageData(result[start:end], page, perPage, len(result))); err != nil {
		http.Error(response, "image page rendering failed", http.StatusInternalServerError)
	}
}

func (handler *itemHandler) itemCollection(response http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.List(request.Context())
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPost:
		var input itemInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.CreateWithCategories(request.Context(), input.Name, input.Description, input.Price, input.CategoryIDs)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusCreated, result)
	default:
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (handler *itemHandler) itemResource(response http.ResponseWriter, request *http.Request) {
	id, err := resourceID(request.URL.Path, "/api/items/")
	if err != nil {
		writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid item id"})
		return
	}
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.Get(request.Context(), id)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPut, http.MethodPatch:
		var input itemInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.Update(request.Context(), id, input.Name, input.Description, input.Price)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodDelete:
		if err := handler.service.Delete(request.Context(), id); err != nil {
			writeItemError(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	default:
		response.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (handler *itemHandler) imageCollection(response http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.ListImages(request.Context())
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPost:
		var input imageInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.CreateImage(request.Context(), input.ItemID, input.URL)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusCreated, result)
	default:
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (handler *itemHandler) imageResource(response http.ResponseWriter, request *http.Request) {
	id, err := resourceID(request.URL.Path, "/api/item-images/")
	if err != nil {
		writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid image id"})
		return
	}
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.GetImage(request.Context(), id)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPut, http.MethodPatch:
		var input imageInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.UpdateImage(request.Context(), id, input.URL)
		if err != nil {
			writeItemError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodDelete:
		if err := handler.service.DeleteImage(request.Context(), id); err != nil {
			writeItemError(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	default:
		response.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type itemInput struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Price       items.Money `json:"price"`
	ImageURLs   []string    `json:"image_urls"`
	CategoryIDs []uint64    `json:"category_ids"`
}

type imageInput struct {
	ItemID uint64 `json:"item_id"`
	URL    string `json:"image_url"`
}

func parseItemForm(request *http.Request) (itemInput, error) {
	price, err := items.ParseMoney(request.FormValue("price"))
	if err != nil {
		return itemInput{}, errors.New("valid price is required")
	}

	return itemInput{Name: request.FormValue("name"), Description: request.FormValue("description"), Price: price, ImageURLs: request.Form["image_url"]}, nil
}

func parseUploads(request *http.Request) ([]storage.Object, error) {
	if err := request.ParseMultipartForm(20 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return nil, nil
		}
		return nil, err
	}
	files := request.MultipartForm.File["image"]
	uploads := make([]storage.Object, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		uploads = append(uploads, storage.Object{Name: header.Filename, ContentType: header.Header.Get("Content-Type"), Reader: file})
	}
	return uploads, nil
}

func resourceID(path, prefix string) (uint64, error) {
	value := strings.TrimPrefix(path, prefix)
	if value == "" || strings.Contains(value, "/") {
		return 0, errors.New("invalid id")
	}
	return strconv.ParseUint(value, 10, 64)
}

func itemStatus(err error) int {
	switch {
	case errors.Is(err, items.ErrInvalidName), errors.Is(err, items.ErrInvalidDescription),
		errors.Is(err, items.ErrInvalidPrice), errors.Is(err, items.ErrInvalidMoney),
		errors.Is(err, items.ErrInvalidImageURL), errors.Is(err, items.ErrImageNotOwned):
		return http.StatusBadRequest
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func itemPageError(err error, fallback string) string {
	if itemStatus(err) == http.StatusBadRequest {
		return err.Error()
	}
	return fallback
}

func writeItemError(response http.ResponseWriter, err error) {
	status := itemStatus(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "item operation failed"
	}
	writeJSONValue(response, status, map[string]string{"error": message})
}
