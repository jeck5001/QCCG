//go:build headless

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"qccg/account"
	"qccg/internal/bridge"
	"qccg/internal/cosy"
	"qccg/logger"
)

// Headless mode: only starts bridge HTTP service without Wails GUI.
// Build with: go build -tags headless -o qccg-bridge .
//
// Environment variables:
//
//	QODER_PAT          - Qoder PAT token (required)
//	QCCG_REGION        - Region: "global" or "cn" (default: global)
//	QCCG_PORT          - Bridge port (default: 8963)
//	QCCG_BRIDGE_TOKEN  - Auth token (default: "qccg")
//	QCCG_LISTEN        - Listen address (default: "0.0.0.0" for Docker)
//	QCCG_DATA_DIR      - Data directory (default: ~/.qccg, mount volume in Docker)

func main() {
	region := account.NormalizeRegion(os.Getenv("QCCG_REGION"))
	port := envInt("QCCG_PORT", 8963)
	bridgeToken := envOrDefault("QCCG_BRIDGE_TOKEN", "qccg")
	listenAddr := envOrDefault("QCCG_LISTEN", "0.0.0.0")
	dataDir := os.Getenv("QCCG_DATA_DIR")
	if dataDir != "" {
		os.Setenv("HOME", dataDir)
	}

	logDir := filepath.Join(dataDirOrDefault(), ".qccg", "logs")
	if err := logger.InitFile(logDir); err != nil {
		fmt.Fprintf(os.Stderr, "[logger] init file sink failed: %v\n", err)
	}

	logLevel := envOrDefault("QCCG_LOG_LEVEL", "info")
	logger.SetLevel(logLevel)

	logger.Info("QCCG Bridge starting (headless mode)")
	logger.Info("Config: region=%s, port=%d, listen=%s, dataDir=%s", region, port, listenAddr, dataDirOrDefault())

	tmpl := string(basePromptRaw)
	for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
		tmpl = strings.ReplaceAll(tmpl, ukey, cosy.NewUUID())
	}
	tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", cosy.UnixMs()))
	var templateBase map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl), &templateBase); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse baseprompt.json: %v\n", err)
		os.Exit(1)
	}

	pat, err := resolveHeadlessToken(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve bridge token: %v\n", err)
		os.Exit(1)
	}

	mux := newHeadlessMux(pat, region, templateBase, nil, port, bridgeToken)
	handler := loggingMiddleware(mux)

	addr := fmt.Sprintf("%s:%d", listenAddr, port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received signal: %v, shutting down...", sig)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server shutdown error: %v", err)
		}
	}()

	logger.Info("Bridge listening on %s", addr)
	fmt.Printf("QCCG Bridge listening on %s (region=%s)\n", addr, region)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}

	logger.Info("Bridge stopped")
	logger.Close()
}

func resolveHeadlessToken(ctx context.Context) (string, error) {
	if pat := os.Getenv("QODER_PAT"); pat != "" {
		return pat, nil
	}
	acct, err := account.GetActive()
	if err != nil || acct == nil {
		return "", err
	}
	return account.GetSecret(acct.ID)
}

func newHeadlessMux(
	pat string,
	region account.Region,
	templateBase map[string]interface{},
	b *bridge.Bridge,
	port int,
	bridgeToken string,
) *http.ServeMux {
	if b == nil && pat != "" {
		if created, err := bridge.NewBridge(pat, region, templateBase); err == nil {
			b = created
			headlessBridge = created
		} else {
			logger.Error("Failed to create bridge: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", dynamicBridgeHandler(bridgeToken, func(b *bridge.Bridge) http.HandlerFunc {
		return b.HandleChatCompletions
	}))
	mux.HandleFunc("/v1/messages", dynamicBridgeHandler(bridgeToken, func(b *bridge.Bridge) http.HandlerFunc {
		return b.HandleClaudeMessages
	}))
	mux.HandleFunc("/v1/models", dynamicBridgeHandler(bridgeToken, func(b *bridge.Bridge) http.HandlerFunc {
		return b.HandleListModels
	}))
	mux.HandleFunc("/v1/responses", dynamicBridgeHandler(bridgeToken, func(b *bridge.Bridge) http.HandlerFunc {
		return b.HandleCodexResponses
	}))
	mux.HandleFunc("/health", healthHandler(port, region))

	mux.Handle("/api/v1/", apiHandler(pat, region, templateBase))
	mux.Handle("/", webUIHandler())
	return mux
}

func dynamicBridgeHandler(token string, pick func(*bridge.Bridge) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if headlessBridge == nil {
			bridgeUnavailableHandler(w, r)
			return
		}
		authMiddleware(token, pick(headlessBridge))(w, r)
	}
}

func bridgeUnavailableHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprint(w, `{"error":{"message":"bridge is not configured; add and activate an account in the web UI","type":"bridge_unavailable"}}`)
}

func authMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		expected := "Bearer " + token
		if token != "qccg" && authHeader != expected {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":{"message":"invalid auth token","type":"auth_error"}}`)
			return
		}
		next(w, r)
	}
}

func healthHandler(port int, region account.Region) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","port":%d,"region":"%s"}`, port, region)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("[HTTP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func dataDirOrDefault() string {
	if d := os.Getenv("QCCG_DATA_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return home
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// webUIHandler serves the embedded frontend static files for the Web UI.
func webUIHandler() http.Handler {
	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(dist))
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		serveEmbeddedFile(w, r, dist, "index.html")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "ui" || path == "ui/" {
			serveIndex(w, r)
			return
		}

		f, err := dist.Open(path)
		if err != nil {
			serveIndex(w, r)
			return
		}
		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}

func serveEmbeddedFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	stat, err := fs.Stat(fsys, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), bytes.NewReader(data))
}
