/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

// iconPath is where the setup comment's action link icons are served.
const iconPath = "/v1/static/icons/"

// iconFS holds the action link icons.
//
//go:embed static/icons/*.svg
var iconFS embed.FS

// --- GET /v1/static/icons/{name} ---.
// GitHub fetches these through its image proxy, which only passes them on
// when they are served as image/svg+xml.
func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if path.Ext(name) != ".svg" {
		http.NotFound(w, r)
		return
	}

	file := path.Join("static/icons", name)
	if _, err := fs.Stat(iconFS, file); err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeFileFS(w, r, iconFS, file) //nolint:gosec // file can only name an embedded icon.
}
