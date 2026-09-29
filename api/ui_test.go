// Copyright 2026 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiRouter "github.com/tsuru/tsuru/api/router"
	"github.com/urfave/negroni/v3"
)

func newTestUIHandler(t *testing.T, configFile string) *uiHandler {
	t.Helper()
	return newTestUIHandlerIn(t, t.TempDir(), configFile)
}

func newTestUIHandlerIn(t *testing.T, dir, configFile string) *uiHandler {
	t.Helper()
	files := map[string]string{
		"index.html":              "root index",
		"favicon.ico":             "icon",
		"config.js":               "export default {prefix: '/ui'};",
		"nginx.conf":              "server {}",
		"static/js/main.abc.js":   "console.log('main');",
		"docs/index.html":         "docs index",
		"legacy/index.htm":        "legacy index",
		"about.html":              "about page",
		"static/css/main.def.css": "body {}",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	h, err := newUIHandler(dir, configFile)
	require.NoError(t, err)
	return h
}

func serveUI(h http.Handler, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func TestUIHandlerServesLikeNginxTryFiles(t *testing.T) {
	h := newTestUIHandler(t, "")
	tests := []struct {
		target string
		body   string
	}{
		{"/ui/", "root index"},
		{"/ui/favicon.ico", "icon"},
		{"/ui/static/js/main.abc.js", "console.log('main');"},
		{"/ui/docs", "docs index"},
		{"/ui/docs/", "docs index"},
		{"/ui/legacy", "legacy index"},
		{"/ui/about", "about page"},
		{"/ui/apps/myapp", "root index"},
		{"/ui/apps/myapp?tab=logs", "root index"},
		{"/ui/nginx.conf", "root index"},
		{"/ui/../../../etc/passwd", "root index"},
		{"/ui/%2e%2e/%2e%2e/etc/passwd", "root index"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			recorder := serveUI(h, http.MethodGet, tt.target)
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, tt.body, recorder.Body.String())
		})
	}
}

func TestUIHandlerContentTypeAndCaching(t *testing.T) {
	h := newTestUIHandler(t, "")
	recorder := serveUI(h, http.MethodGet, "/ui/static/js/main.abc.js")
	assert.Equal(t, "text/javascript; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	assert.NotEmpty(t, recorder.Header().Get("ETag"))

	recorder = serveUI(h, http.MethodGet, "/ui/apps/myapp")
	assert.Equal(t, "text/html; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))

	recorder = serveUI(h, http.MethodGet, "/ui/favicon.ico")
	assert.Empty(t, recorder.Header().Get("Cache-Control"))
}

func TestUIHandlerConditionalRequest(t *testing.T) {
	h := newTestUIHandler(t, "")
	etag := serveUI(h, http.MethodGet, "/ui/favicon.ico").Header().Get("ETag")
	req := httptest.NewRequest(http.MethodGet, "/ui/favicon.ico", nil)
	req.Header.Set("If-None-Match", etag)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	assert.Equal(t, http.StatusNotModified, recorder.Code)
}

func TestUIHandlerMissingStaticAssetIsNotFound(t *testing.T) {
	h := newTestUIHandler(t, "")
	recorder := serveUI(h, http.MethodGet, "/ui/static/js/main.old.js")
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestUIHandlerRedirectsPrefixToSlash(t *testing.T) {
	h := newTestUIHandler(t, "")
	recorder := serveUI(h, http.MethodGet, "/ui?code=123")
	assert.Equal(t, http.StatusMovedPermanently, recorder.Code)
	assert.Equal(t, "/ui/?code=123", recorder.Header().Get("Location"))
}

func TestUIHandlerHead(t *testing.T) {
	h := newTestUIHandler(t, "")
	recorder := serveUI(h, http.MethodHead, "/ui/")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Body.String())
}

func TestUIHandlerConfigFromBuild(t *testing.T) {
	h := newTestUIHandler(t, "")
	recorder := serveUI(h, http.MethodGet, "/ui/config.js")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "export default {prefix: '/ui'};", recorder.Body.String())
	assert.Equal(t, "text/javascript; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
}

func TestUIHandlerConfigMissingIsNotFound(t *testing.T) {
	dir := t.TempDir()
	h := newTestUIHandlerIn(t, dir, "")
	require.NoError(t, os.Remove(filepath.Join(dir, "config.js")))
	recorder := serveUI(h, http.MethodGet, "/ui/config.js")
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestUIHandlerConfigFileOverride(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "ui-config.js")
	require.NoError(t, os.WriteFile(configFile, []byte("export default {server: ''};"), 0o644))
	h := newTestUIHandler(t, configFile)

	recorder := serveUI(h, http.MethodGet, "/ui/config.js")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "export default {server: ''};", recorder.Body.String())
	assert.Equal(t, "text/javascript; charset=utf-8", recorder.Header().Get("Content-Type"))

	// Read on every request, so changes apply without a restart.
	require.NoError(t, os.WriteFile(configFile, []byte("export default {server: '/api'};"), 0o644))
	recorder = serveUI(h, http.MethodGet, "/ui/config.js")
	assert.Equal(t, "export default {server: '/api'};", recorder.Body.String())

	require.NoError(t, os.Remove(configFile))
	recorder = serveUI(h, http.MethodGet, "/ui/config.js")
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestNewUIHandlerErrors(t *testing.T) {
	_, err := newUIHandler(filepath.Join(t.TempDir(), "missing"), "")
	assert.ErrorContains(t, err, "unable to open ui dir")

	_, err = newUIHandler(t.TempDir(), "")
	assert.ErrorContains(t, err, "has no index.html")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), nil, 0o644))
	_, err = newUIHandler(dir, filepath.Join(dir, "missing.js"))
	assert.ErrorContains(t, err, "unable to read ui config file")
}

func TestUIRegisteredOnRouter(t *testing.T) {
	h := newTestUIHandler(t, "")
	m := apiRouter.NewRouter()
	m.Add("1.0", http.MethodGet, "/apps", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("api"))
	}))
	registerUI(m, h)
	n := negroni.New()
	n.UseHandler(m)
	n.Use(negroni.HandlerFunc(errorHandlingMiddleware))
	n.Use(negroni.HandlerFunc(setVersionHeadersMiddleware))
	n.Use(negroni.HandlerFunc(authTokenMiddleware))
	n.UseHandler(http.HandlerFunc(runDelayedHandler))

	for _, tt := range []struct{ target, body string }{
		{"/ui/", "root index"},
		{"/ui/apps/x", "root index"},
		{"/ui/static/js/main.abc.js", "console.log('main');"},
		{"/apps", "api"},
	} {
		recorder := serveUI(n, http.MethodGet, tt.target)
		assert.Equal(t, tt.body, recorder.Body.String(), tt.target)
	}

	t.Run("redirect keeps query clean", func(t *testing.T) {
		recorder := serveUI(n, http.MethodGet, "/ui?code=123")
		assert.Equal(t, http.StatusMovedPermanently, recorder.Code)
		assert.Equal(t, "/ui/?code=123", recorder.Header().Get("Location"))
	})
	t.Run("head", func(t *testing.T) {
		recorder := serveUI(n, http.MethodHead, "/ui/static/js/main.abc.js")
		assert.Equal(t, http.StatusOK, recorder.Code)
	})
	t.Run("other methods are not found", func(t *testing.T) {
		recorder := serveUI(n, http.MethodPost, "/ui/")
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
	t.Run("versioned path", func(t *testing.T) {
		recorder := serveUI(n, http.MethodGet, "/1.0/ui/apps/x")
		t.Logf("/1.0/ui/apps/x -> %d %q", recorder.Code, recorder.Body.String())
	})
}
