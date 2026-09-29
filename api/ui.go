// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/tsuru/config"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/tsuru/tsuru/log"
)

const uiPrefix = "/ui"

// uiHandler serves the tsuru-ui static build under /ui, mirroring the nginx
// config shipped with it: try_files $uri $uri/ $uri.html /index.html.
type uiHandler struct {
	root       fs.FS
	configFile string
}

// newUIHandler serves the build in dir. When configFile is not empty it is
// served as /ui/config.js instead of the one in dir, read on every request so
// it can be changed without restarting the API.
func newUIHandler(dir, configFile string) (*uiHandler, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to open ui dir: %w", err)
	}
	if _, err = root.Stat("index.html"); err != nil {
		return nil, fmt.Errorf("ui dir %q has no index.html: %w", dir, err)
	}
	if configFile != "" {
		if _, err = os.Stat(configFile); err != nil {
			return nil, fmt.Errorf("unable to read ui config file: %w", err)
		}
	}
	return &uiHandler{root: root.FS(), configFile: configFile}, nil
}

func uiHandlerFromConfig() (*uiHandler, error) {
	if enabled, _ := config.GetBool("ui:enabled"); !enabled {
		return nil, nil
	}
	dir, err := config.GetString("ui:dir")
	if err != nil {
		return nil, errors.New("ui:dir is required when ui:enabled is true")
	}
	configFile, _ := config.GetString("ui:config-file")
	return newUIHandler(dir, configFile)
}

// registerUI binds the ui handler on the router, like any other route.
func registerUI(m *apiRouter.DelayedRouter, h *uiHandler) {
	// HEAD is registered explicitly: the router matches methods strictly, so
	// it would answer 404 to a HEAD on a file that is there. Other methods
	// get the router's own 404, which is answer enough for a static tree.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		m.Add("1.0", method, uiPrefix, h)
		m.Add("1.0", method, uiPrefix+"/{path:.*}", h)
	}
}

func (h *uiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == uiPrefix {
		target := uiPrefix + "/"
		if q := clientQuery(r); q != "" {
			target += "?" + q
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	name := strings.TrimPrefix(path.Clean(strings.TrimPrefix(r.URL.Path, uiPrefix)), "/")
	if name == "config.js" {
		h.serveConfig(w, r)
		return
	}
	if name == "nginx.conf" {
		// Part of the build for the tsuru static deploy, not meant to be served.
		name = ""
	}
	if name == "" {
		h.serveFile(w, r, "index.html")
		return
	}
	candidates := []string{name, path.Join(name, "index.html"), path.Join(name, "index.htm"), name + ".html"}
	for _, candidate := range candidates {
		if h.isFile(candidate) {
			h.serveFile(w, r, candidate)
			return
		}
	}
	// Assets are content hashed, so a missing one is most likely a stale
	// reference from before a deploy: answering with index.html would only
	// surface as a confusing MIME type error in the browser.
	if strings.HasPrefix(name, "static/") {
		http.NotFound(w, r)
		return
	}
	h.serveFile(w, r, "index.html")
}

// clientQuery drops the variables the router injects into RawQuery when it
// matches a route, keeping only what the client actually sent.
func clientQuery(r *http.Request) string {
	values := r.URL.Query()
	for key := range values {
		if strings.HasPrefix(key, ":") {
			delete(values, key)
		}
	}
	return values.Encode()
}

func (h *uiHandler) isFile(name string) bool {
	info, err := fs.Stat(h.root, name)
	return err == nil && info.Mode().IsRegular()
}

func (h *uiHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := h.root.Open(name)
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	defer f.Close()
	switch {
	case strings.HasPrefix(name, "static/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("Cache-Control", "no-cache")
	}
	serveContent(w, r, name, f)
}

func (h *uiHandler) serveConfig(w http.ResponseWriter, r *http.Request) {
	var (
		f   fs.File
		err error
	)
	if h.configFile != "" {
		f, err = os.Open(h.configFile)
	} else {
		f, err = h.root.Open("config.js")
	}
	if err != nil {
		h.serveError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	serveContent(w, r, "config.js", f)
}

func (h *uiHandler) serveError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	log.Errorf("unable to serve ui file %q: %v", r.URL.Path, err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func serveContent(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	info, err := f.Stat()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	content, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	// Same ETag format as nginx, so conditional requests keep working.
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime().Unix(), info.Size()))
	http.ServeContent(w, r, name, info.ModTime(), content)
}
