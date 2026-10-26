package web

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var embedded embed.FS

func Files() fs.FS {
	sub, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}
