//go:build !headless

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"qccg/logger"
)

// ClientConfig 是返回前端的客户端配置状态。
//   - ConfigPath/BaseURL/EnvVars 仅作展示
//   - Applied 表示「已经被 qccg 标注过」(由 Marker 字段判断，避免误判用户原本就有的配置)
//   - Model 是当前配置文件里读出来的「主力」模型名（仅展示）
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

// ClientConfigFile 是返回前端的「主配置文件原文 + 路径」结构，用于编辑器展示
type ClientConfigFile struct {
	Path       string             `json:"path"`
	Content    string             `json:"content"`
	Format     string             `json:"format"` // "json" / "toml" / "dotenv"
	Existed    bool               `json:"existed"`
	ExtraFiles []ClientConfigFile `json:"extra_files,omitempty"`
}

func (a *App) effectiveToken() string {
	if a.bridgeToken != "" {
		return a.bridgeToken
	}
	return defaultQoderAPIKey
}

func (a *App) GetClientConfigs() []ClientConfig {
	port := a.bridgePort
	token := a.effectiveToken()
	home, _ := os.UserHomeDir()

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
			EnvVars:    "# Codex 不依赖环境变量，配置写入 ~/.codex/config.toml + auth.json 即可",
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

func (a *App) ApplyClientConfig(clientType, model string) error {
	port := a.bridgePort
	token := a.effectiveToken()
	home, err := os.UserHomeDir()
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

// BackupClientConfigFile 在保存前把原文件备份到 ~/.qccg/backups/<type>_config.bak
func (a *App) BackupClientConfigFile(clientType string) error {
	home, err := os.UserHomeDir()
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

// HasClientConfigBackup 返回指定 client 是否存在备份文件
func (a *App) HasClientConfigBackup(clientType string) bool {
	home, _ := os.UserHomeDir()
	return hasBackupFile(home, clientType)
}

// RestoreClientConfigFile 把备份文件还原回原路径，还原后删除备份
func (a *App) RestoreClientConfigFile(clientType string) error {
	home, err := os.UserHomeDir()
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

func (a *App) ReadClientConfigFile(clientType string) (*ClientConfigFile, error) {
	home, err := os.UserHomeDir()
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
	result := mainFile
	if clientType == "codex" {
		extraPath := filepath.Join(home, ".codex", "auth.json")
		extra, err := readSingleClientConfigFile(extraPath, "json")
		if err != nil {
			return nil, err
		}
		result.ExtraFiles = []ClientConfigFile{extra}
	}
	return &result, nil
}

func (a *App) SaveClientConfigFile(clientType, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path, format := mainConfigPath(home, clientType)
	if path == "" {
		return fmt.Errorf("unknown client type: %s", clientType)
	}
	if !a.HasClientConfigBackup(clientType) {
		if err := a.BackupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	logger.Info("Saved %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func (a *App) SaveAdditionalClientConfigFile(clientType, path, format, content string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if clientType != "codex" {
		return fmt.Errorf("client type %s has no additional config files", clientType)
	}
	expectedPath := filepath.Join(home, ".codex", "auth.json")
	if path != expectedPath {
		return fmt.Errorf("unsupported additional config path: %s", path)
	}
	if !a.HasClientConfigBackup(clientType) {
		if err := a.BackupClientConfigFile(clientType); err != nil {
			logger.Info("Backup skipped for %s: %v", clientType, err)
		}
	}
	if err := validateConfigContent(format, content); err != nil {
		return err
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	logger.Info("Saved additional %s config file: %s (%d bytes)", clientType, path, len(content))
	return nil
}

func (a *App) RemoveClientConfig(clientType string) error {
	home, err := os.UserHomeDir()
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
