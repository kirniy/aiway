package webui

import "embed"

//go:embed dist dist/* dist/assets/*
var Files embed.FS
