package httpapi

import (
	"net/http"
	"strconv"
)

type pageData struct {
	Items      any
	Page       int
	PerPage    int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

func paginate(request *http.Request, total int) (page, perPage, start, end int) {
	page = queryInt(request, "page", 1)
	perPage = queryInt(request, "per_page", 10)
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}
	start = (page - 1) * perPage
	if start > total {
		start = total
	}
	end = start + perPage
	if end > total {
		end = total
	}
	return
}

func queryInt(request *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(request.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func newPageData(items any, page, perPage, total int) pageData {
	totalPages := (total + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}
	return pageData{
		Items: items, Page: page, PerPage: perPage, TotalPages: totalPages,
		HasPrev: page > 1, HasNext: page < totalPages,
		PrevPage: page - 1, NextPage: page + 1,
	}
}
