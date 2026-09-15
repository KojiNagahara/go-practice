package view

import "embed"

// Files contains the application HTML templates.
//
//go:embed categories.html categories.css items.html item_images.html edit_item.html items.css shop.html login.html register.html
var Files embed.FS
