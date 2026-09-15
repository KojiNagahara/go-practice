package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"go-practice/internal/categories"
	"go-practice/view"
)

type categoryHandler struct {
	service  *categories.Service
	template *template.Template
}

func newCategoryHandler(service *categories.Service) *categoryHandler {
	return &categoryHandler{
		service:  service,
		template: template.Must(template.ParseFS(view.Files, "categories.html")),
	}
}

func (handler *categoryHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/categories", handler.page)
	mux.HandleFunc("/categories/", handler.page)
	mux.HandleFunc("/api/categories", handler.collection)
	mux.HandleFunc("/api/categories/", handler.item)
}

func categoryStylesheetHandler(response http.ResponseWriter, request *http.Request) {
	stylesheet, err := view.Files.ReadFile("categories.css")
	if err != nil {
		http.Error(response, "stylesheet unavailable", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/css; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(stylesheet)
}

func (handler *categoryHandler) page(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/categories" && request.URL.Path != "/categories/" {
		http.NotFound(response, request)
		return
	}
	if request.Method == http.MethodPost {
		if _, err := handler.service.Create(request.Context(), request.FormValue("name")); err != nil {
			http.Error(response, categoryPageError(err, "category creation failed"), categoryStatus(err))
			return
		}
		http.Redirect(response, request, "/categories", http.StatusSeeOther)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := handler.service.List(request.Context())
	if err != nil {
		http.Error(response, "category loading failed", categoryStatus(err))
		return
	}
	page, perPage, start, end := paginate(request, len(result))
	if err := handler.template.Execute(response, newPageData(result[start:end], page, perPage, len(result))); err != nil {
		http.Error(response, "category page rendering failed", http.StatusInternalServerError)
	}
}

func (handler *categoryHandler) collection(response http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.List(request.Context())
		if err != nil {
			writeCategoryError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.Create(request.Context(), input.Name)
		if err != nil {
			writeCategoryError(response, err)
			return
		}
		writeJSONValue(response, http.StatusCreated, result)
	default:
		response.Header().Set("Allow", "GET, POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (handler *categoryHandler) item(response http.ResponseWriter, request *http.Request) {
	id, err := categoryID(request.URL.Path)
	if err != nil {
		writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid category id"})
		return
	}
	switch request.Method {
	case http.MethodGet:
		result, err := handler.service.Get(request.Context(), id)
		if err != nil {
			writeCategoryError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodPut, http.MethodPatch:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			writeJSONValue(response, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := handler.service.Update(request.Context(), id, input.Name)
		if err != nil {
			writeCategoryError(response, err)
			return
		}
		writeJSONValue(response, http.StatusOK, result)
	case http.MethodDelete:
		if err := handler.service.Delete(request.Context(), id); err != nil {
			writeCategoryError(response, err)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	default:
		response.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func categoryID(path string) (uint64, error) {
	value := strings.TrimPrefix(path, "/api/categories/")
	if value == "" || strings.Contains(value, "/") {
		return 0, errors.New("invalid id")
	}
	return strconv.ParseUint(value, 10, 64)
}

func categoryStatus(err error) int {
	if errors.Is(err, categories.ErrInvalidName) {
		return http.StatusBadRequest
	}
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func categoryPageError(err error, fallback string) string {
	if errors.Is(err, categories.ErrInvalidName) {
		return err.Error()
	}
	return fallback
}

func writeCategoryError(response http.ResponseWriter, err error) {
	status := categoryStatus(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "category operation failed"
	}
	writeJSONValue(response, status, map[string]string{"error": message})
}

func writeJSONValue(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
