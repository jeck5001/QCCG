//go:build headless

package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// In headless (Docker) mode, we use file-based secret storage instead of OS keychain.
// Secrets are stored in ~/.qccg/secrets.json (or QCCG_DATA_DIR/.qccg/secrets.json).

var secretMu sync.Mutex

func secretsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".qccg")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "secrets.json"), nil
}

func loadSecrets() (map[string]string, error) {
	path, err := secretsFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func saveSecrets(m map[string]string) error {
	path, err := secretsFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func SaveSecret(accountID, secret string) error {
	secretMu.Lock()
	defer secretMu.Unlock()
	m, err := loadSecrets()
	if err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}
	m[accountID] = secret
	return saveSecrets(m)
}

func GetSecret(accountID string) (string, error) {
	secretMu.Lock()
	defer secretMu.Unlock()
	m, err := loadSecrets()
	if err != nil {
		return "", fmt.Errorf("get secret %s: %w", accountID, err)
	}
	s, ok := m[accountID]
	if !ok {
		return "", fmt.Errorf("secret not found for %s", accountID)
	}
	return s, nil
}

func DeleteSecret(accountID string) error {
	secretMu.Lock()
	defer secretMu.Unlock()
	m, err := loadSecrets()
	if err != nil {
		return fmt.Errorf("delete secret %s: %w", accountID, err)
	}
	delete(m, accountID)
	return saveSecrets(m)
}
