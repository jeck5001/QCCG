//go:build headless

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"qccg/account"
	"qccg/logger"
)

// client_config_headless.go provides standalone wrappers around client config
// functionality for headless (Docker) mode. In desktop mode these are methods
// on App; here they are free functions called by the REST API handlers.

// effectiveToken returns the bridge auth token from settings.
func effectiveToken() string {
	settings, err := account.LoadSettings()
	if err != nil {
		return defaultQoderAPIKey
	}
	if settings.BridgeToken != "" {
		return settings.BridgeToken
	}
	return defaultQoderAPIKey
}

func effectivePort() int {
	settings, err := account.LoadSettings()
	if err != nil {
		return 8963
	}
	return settings.Port
}

// ClientConfig mirrors the desktop type for JSON responses.
type ClientConfig struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	ConfigPath string `json:"config_path"`
	BaseURL    string `json:"base_url"`
	EnvVars    string `json:"env_vars"`
	Model      string `json:"model"`
	Applied    bool   `json:"applied"`
	Error      string `json:"error,omitempty"`
}

// ClientConfigFile mirrors the desktop type for JSON responses.
type ClientConfigFile struct {
	Path       string             `json:"path"`
	Content    string             `json:"content"`
	Format     string             `json:"format"`
	Existed    bool               `json:"existed"`
	ExtraFiles []ClientConfigFile `json:"extra_files,omitempty"`
}

func getClientConfigs() []ClientConfig {
	port := effectivePort()
	token := effectiveToken()
	home, _ := dataDir()

	configs := []ClientConfig{
		{
			Type:       "claude",
			Name:       "Claude Code",
			Icon:       "🧠",
			ConfigPath: filepath.Join(home, ".claude", "settings.json"),
			BaseURL:    bridgeBaseURL(port),
			EnvVars:    fmt.Sprintf(`export ANTHROPIC_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export ANTHROPIC_AUTH_TOKEN="%s"`, port, token),
		},
		{
			Type:       "codex",
			Name:       "Codex CLI",
			Icon:       "⚡",
			ConfigPath: filepath.Join(home, ".codex", "config.toml"),
			BaseURL:    bridgeBaseURL(port) + codexProviderURL,
			EnvVars:    "# Codex doesn't use env vars; config in ~/.codex/config.toml + auth.json",
		},
		{
			Type:       "gemini",
			Name:       "Gemini CLI",
			Icon:       "🌟",
			ConfigPath: filepath.Join(home, ".gemini", ".env"),
			BaseURL:    bridgeBaseURL(port),
			EnvVars:    fmt.Sprintf(`export GOOGLE_GEMINI_BASE_URL="http://127.0.0.1:%d"`+"\n"+`export GEMINI_API_KEY="%s"`, port, token),
		},
	}

	for i := range configs {
		cfg := &configs[i]
		applied, model, err := readClientStatus(cfg.Type, home, port, token)
		cfg.Applied = applied
		cfg.Model = model
		if err != nil {
			cfg.Error = err.Error()
		}
	}
	return configs
}

func readClientConfigFile(clientType string) (*ClientConfigFile, error) {
	home, err := dataDir()
	if err != nil {
		return nil, err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return nil, fmt.Errorf("unknown client type: %s", clientType)
	}
	mainFile, err := readSingleClientConfigFile(path, format)
	if err != nil {
		return nil, err
	}
	result := ClientConfigFile{
		Path:    mainFile.Path,
		Content: mainFile.Content,
		Format:  mainFile.Format,
		Existed: mainFile.Existed,
	}
	if clientType == "codex" {
		extraPath := filepath.Join(home, ".codex", "auth.json")
		extra, err := readSingleClientConfigFile(extraPath, "json")
		if err != nil {
			return nil, err
		}
		result.ExtraFiles = []ClientConfigFile{{
			Path:    extra.Path,
			Content: extra.Content,
			Format:  extra.Format,
			Existed: extra.Existed,
		}}
	}
	return &result, nil
}

func saveClientConfigFile(clientType, content string) error {
	home, err := dataDir()
	if err != nil {
		return err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	if !hasClientConfigBackup(clientType) {
		if err := backupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	logger.Info("Saved %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func saveAdditionalClientConfigFile(clientType, path, format, content string) error {
	if clientType != "codex" {
		return fmt.Errorf("client type %s has no additional config files", clientType)
	}
	home, err := dataDir()
	if err != nil {
		return err
	}
	expectedPath := filepath.Join(home, ".codex", "auth.json")
	if path != expectedPath {
		return fmt.Errorf("unsupported additional config path: %s", path)
	}
	if !hasClientConfigBackup(clientType) {
		if err := backupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	logger.Info("Saved additional %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func applyClientConfig(clientType, model string) error {
	port := effectivePort()
	token := effectiveToken()
	home, err := dataDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	switch clientType {
	case "claude":
		return writeClaudeConfig(home, port, token, model)
	case "codex":
		return writeCodexConfig(home, port, token, model)
	case "gemini":
		return writeGeminiConfig(home, port, token, model)
	}
	return fmt.Errorf("unknown client type: %s", clientType)
}

func removeClientConfig(clientType string) error {
	home, err := dataDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	switch clientType {
	case "claude":
		return removeClaudeConfig(home)
	case "codex":
		return removeCodexConfig(home)
	case "gemini":
		return removeGeminiConfig(home)
	}
	return fmt.Errorf("unknown client type: %s", clientType)
}

func backupClientConfigFile(clientType string) error {
	home, err := dataDir()
	if err != nil {
		return err
	}
	src, _ := mainConfigPath(home, clientType)
	if src == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		dst := backupPath(home, clientType)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := atomicWriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}

	if clientType == "codex" {
		authSrc := filepath.Join(home, ".codex", "auth.json")
		authBak := codexAuthBackupPath(home)
		missingMarker := codexAuthMissingMarkerPath(home)
		authData, authErr := os.ReadFile(authSrc)
		if authErr != nil {
			if !os.IsNotExist(authErr) {
				return authErr
			}
			if err := os.MkdirAll(filepath.Dir(missingMarker), 0o755); err != nil {
				return err
			}
			if err := atomicWriteFile(missingMarker, []byte("missing"), 0o644); err != nil {
				return err
			}
			_ = os.Remove(authBak)
		} else {
			if err := os.MkdirAll(filepath.Dir(authBak), 0o755); err != nil {
				return err
			}
			if err := atomicWriteFile(authBak, authData, 0o644); err != nil {
				return err
			}
			_ = os.Remove(missingMarker)
		}
	}
	return nil
}

func hasClientConfigBackup(clientType string) bool {
	home, _ := dataDir()
	return hasBackupFile(home, clientType)
}

func restoreClientConfigFile(clientType string) error {
	home, err := dataDir()
	if err != nil {
		return err
	}
	dst, _ := mainConfigPath(home, clientType)
	if dst == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	bak := backupPath(home, clientType)
	data, err := os.ReadFile(bak)
	if err != nil {
		return fmt.Errorf("no backup found: %w", err)
	}
	if err := atomicWriteFile(dst, data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(bak)

	if clientType == "codex" {
		authPath := filepath.Join(home, ".codex", "auth.json")
		authBak := codexAuthBackupPath(home)
		missingMarker := codexAuthMissingMarkerPath(home)
		if authData, authErr := os.ReadFile(authBak); authErr == nil {
			if err := atomicWriteFile(authPath, authData, 0o644); err != nil {
				return err
			}
			_ = os.Remove(authBak)
			_ = os.Remove(missingMarker)
		} else if _, markerErr := os.Stat(missingMarker); markerErr == nil {
			_ = os.Remove(authPath)
			_ = os.Remove(missingMarker)
		}
	}

	logger.Info("Restored %s config from backup", clientType)
	return nil
}

// dataDir returns the effective home directory (respects QCCG_DATA_DIR).
func dataDir() (string, error) {
	if d := os.Getenv("QCCG_DATA_DIR"); d != "" {
		return d, nil
	}
	return os.UserHomeDir()
}

// removeAll is a helper to avoid importing os.RemoveAll indirectly.
func removeAll(path string) error {
	return os.RemoveAll(path)
}
