//go:build headless

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"qccg/account"
	"qccg/internal/bridge"
	"qccg/internal/cosy"
	"qccg/internal/updater"
	"qccg/logger"
)

// apiHandler returns a http.Handler that serves the management REST API.
// All endpoints are prefixed with /api/v1/.
func apiHandler(pat string, region account.Region, templateBase map[string]interface{}) http.Handler {
	mux := http.NewServeMux()

	// --- Status & Bridge ---
	mux.HandleFunc("GET /api/v1/status", handleStatus(pat, region, templateBase))
	mux.HandleFunc("POST /api/v1/bridge/start", handleBridgeStart(pat, region, templateBase))
	mux.HandleFunc("POST /api/v1/bridge/stop", handleBridgeStop())

	// --- Accounts ---
	mux.HandleFunc("GET /api/v1/accounts", handleListAccounts)
	mux.HandleFunc("POST /api/v1/accounts", handleAddAccountByPAT)
	mux.HandleFunc("DELETE /api/v1/accounts/{id}", handleDeleteAccount)
	mux.HandleFunc("PUT /api/v1/accounts/{id}/active", handleSetActiveAccount)
	mux.HandleFunc("PUT /api/v1/accounts/reorder", handleReorderAccounts)
	mux.HandleFunc("GET /api/v1/accounts/{id}/quota", handleGetAccountQuota)
	mux.HandleFunc("PUT /api/v1/accounts/{id}/api-mode", handleUpdateAccountAPIMode)

	// --- OAuth ---
	mux.HandleFunc("POST /api/v1/oauth/start", handleStartOAuthLogin)
	mux.HandleFunc("GET /api/v1/oauth/wait/{loginID}", handleWaitOAuthLogin)
	mux.HandleFunc("POST /api/v1/oauth/cancel/{loginID}", handleCancelOAuthLogin)

	// --- Settings ---
	mux.HandleFunc("GET /api/v1/settings", handleGetSettings)
	mux.HandleFunc("PUT /api/v1/settings", handleSaveSettings)

	// --- Client Config ---
	mux.HandleFunc("GET /api/v1/client-configs", handleGetClientConfigs)
	mux.HandleFunc("GET /api/v1/client-configs/{type}", handleReadClientConfigFile)
	mux.HandleFunc("PUT /api/v1/client-configs/{type}", handleSaveClientConfigFile)
	mux.HandleFunc("PUT /api/v1/client-configs/{type}/additional", handleSaveAdditionalClientConfigFile)
	mux.HandleFunc("POST /api/v1/client-configs/{type}/apply", handleApplyClientConfig)
	mux.HandleFunc("DELETE /api/v1/client-configs/{type}", handleRemoveClientConfig)
	mux.HandleFunc("POST /api/v1/client-configs/{type}/backup", handleBackupClientConfigFile)
	mux.HandleFunc("GET /api/v1/client-configs/{type}/backup", handleHasClientConfigBackup)
	mux.HandleFunc("POST /api/v1/client-configs/{type}/restore", handleRestoreClientConfigFile)

	// --- Models ---
	mux.HandleFunc("GET /api/v1/models", handleListQoderModels)

	// --- Logs ---
	mux.HandleFunc("GET /api/v1/logs", handleGetLogsSince)
	mux.HandleFunc("DELETE /api/v1/logs", handleClearLogs)

	// --- Update ---
	mux.HandleFunc("GET /api/v1/version", handleGetVersion)
	mux.HandleFunc("GET /api/v1/update/check", handleCheckUpdate)
	mux.HandleFunc("POST /api/v1/update/apply", handleApplyUpdate)

	// --- Cleanup ---
	mux.HandleFunc("POST /api/v1/cleanup", handleCleanupAllData)

	return mux
}

// ============================================================
// Bridge state (shared across handlers)
// ============================================================

var (
	headlessBridge    *bridge.Bridge
	headlessBridgeSrv *http.Server
)

// ============================================================
// Helpers
// ============================================================

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// ============================================================
// Status & Bridge
// ============================================================

type statusResponse struct {
	Running       bool   `json:"running"`
	Port          int    `json:"port"`
	ActiveAccount string `json:"active_account"`
	APIMode       string `json:"api_mode"`
	Region        string `json:"region"`
}

func handleStatus(pat string, region account.Region, templateBase map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		activeID := ""
		apiMode := ""
		if acct, _ := account.GetActive(); acct != nil {
			activeID = acct.ID
			apiMode = acct.APIMode
		}
		running := headlessBridge != nil
		port := envInt("QCCG_PORT", 8963)
		writeJSON(w, http.StatusOK, statusResponse{
			Running:       running,
			Port:          port,
			ActiveAccount: activeID,
			APIMode:       apiMode,
			Region:        string(region),
		})
	}
}

func handleBridgeStart(pat string, region account.Region, templateBase map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acct, err := account.GetActive()
		if err != nil || acct == nil {
			writeError(w, http.StatusBadRequest, "no active account")
			return
		}
		patToken, err := account.GetSecret(acct.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get secret")
			return
		}
		tmpl := string(basePromptRaw)
		for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
			tmpl = strings.ReplaceAll(tmpl, ukey, cosy.NewUUID())
		}
		tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", cosy.UnixMs()))
		b, err := bridge.NewBridge(patToken, acct.Region, templateBase)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("create bridge: %v", err))
			return
		}
		headlessBridge = b
		writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
	}
}

func handleBridgeStop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		headlessBridge = nil
		writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
	}
}

// ============================================================
// Accounts
// ============================================================

func handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := account.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if accounts == nil {
		accounts = []account.Account{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

type addAccountRequest struct {
	PAT    string `json:"pat"`
	Region string `json:"region"`
}

func handleAddAccountByPAT(w http.ResponseWriter, r *http.Request) {
	var req addAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	region := account.NormalizeRegion(req.Region)
	ep := account.GetEndpoints(region)
	mid := cosy.NewUUID()
	mtoken := cosy.NewBase64Token()
	mtype := cosy.NewHexToken(18)

	jt, err := cosy.ExchangeJobToken(req.PAT, mid, mtoken, mtype, ep.JobTokenURL)
	if err != nil {
		writeError(w, http.StatusUnauthorized, fmt.Sprintf("验证 PAT 失败: %v", err))
		return
	}
	id := account.SanitizeID(bridge.StrVal(jt, "id") + bridge.StrVal(jt, "name"))
	acct := &account.Account{
		ID:       id,
		Name:     bridge.StrVal(jt, "name"),
		Email:    bridge.StrVal(jt, "email"),
		UserType: bridge.StrValDefault(jt, "userType", "personal_standard"),
		Region:   region,
		AuthMode: "pat",
		APIMode:  "openai",
	}
	if err := account.SaveSecret(acct.ID, req.PAT); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := account.Save(acct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acct)
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = account.DeleteSecret(id)
	if err := account.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func handleSetActiveAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := account.SetActive(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := restartHeadlessBridgeForAccount(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func restartHeadlessBridgeForAccount(id string) error {
	acct, err := account.Get(id)
	if err != nil {
		return err
	}
	token, err := account.GetSecret(acct.ID)
	if err != nil {
		return err
	}
	tmpl := string(basePromptRaw)
	for _, ukey := range []string{"{UUID1}", "{UUID2}", "{UUID3}", "{UUID4}", "{UUID5}"} {
		tmpl = strings.ReplaceAll(tmpl, ukey, cosy.NewUUID())
	}
	tmpl = strings.ReplaceAll(tmpl, "{TIME1}", fmt.Sprintf("%d", cosy.UnixMs()))
	var templateBase map[string]interface{}
	if err := json.Unmarshal([]byte(tmpl), &templateBase); err != nil {
		return err
	}
	b, err := bridge.NewBridge(token, acct.Region, templateBase)
	if err != nil {
		return err
	}
	headlessBridge = b
	return nil
}

type reorderRequest struct {
	IDs []string `json:"ids"`
}

func handleReorderAccounts(w http.ResponseWriter, r *http.Request) {
	var req reorderRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := account.Reorder(req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleGetAccountQuota(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct, err := account.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	token, err := account.GetSecret(acct.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get token: "+err.Error())
		return
	}
	deviceToken, _ := bridge.ParseOAuthSecret(token)
	if len(deviceToken) > 3 && deviceToken[:3] == "dt-" {
		token = deviceToken
	} else {
		ep := account.GetEndpoints(acct.Region)
		jt, err := cosy.ExchangeJobToken(token, cosy.NewUUID(), cosy.NewBase64Token(), cosy.NewHexToken(18), ep.JobTokenURL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "exchange token: "+err.Error())
			return
		}
		oauthToken := bridge.StrVal(jt, "securityOauthToken")
		if oauthToken == "" {
			writeError(w, http.StatusInternalServerError, "no securityOauthToken in response")
			return
		}
		token = oauthToken
	}
	quota, err := account.FetchQuota(token, acct.Region)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quota)
}

type updateAPIModeRequest struct {
	APIMode string `json:"api_mode"`
}

func handleUpdateAccountAPIMode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req updateAPIModeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	acct, err := account.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	acct.APIMode = req.APIMode
	if err := account.Save(acct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// OAuth
// ============================================================

func handleStartOAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Region string `json:"region"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	session, err := account.StartLogin(account.NormalizeRegion(req.Region))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func handleWaitOAuthLogin(w http.ResponseWriter, r *http.Request) {
	loginID := r.PathValue("loginID")
	acct, err := account.WaitLogin(loginID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := account.Save(acct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func handleCancelOAuthLogin(w http.ResponseWriter, r *http.Request) {
	loginID := r.PathValue("loginID")
	account.CancelLogin(loginID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ============================================================
// Settings
// ============================================================

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := account.LoadSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var s account.Settings
	if err := readJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logger.SetLevel(s.LogLevel)
	if s.QuotaRefreshInterval > 0 && s.QuotaRefreshInterval < 10 {
		s.QuotaRefreshInterval = 10
	}
	if err := account.SaveSettings(&s); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Client Config — delegated to client_config_headless.go
// ============================================================

func handleGetClientConfigs(w http.ResponseWriter, r *http.Request) {
	configs := getClientConfigs()
	writeJSON(w, http.StatusOK, configs)
}

func handleReadClientConfigFile(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	result, err := readClientConfigFile(clientType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleSaveClientConfigFile(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := saveClientConfigFile(clientType, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleSaveAdditionalClientConfigFile(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	var req struct {
		Path    string `json:"path"`
		Format  string `json:"format"`
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := saveAdditionalClientConfigFile(clientType, req.Path, req.Format, req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleApplyClientConfig(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	var req struct {
		Model string `json:"model"`
	}
	readJSON(r, &req)
	if err := applyClientConfig(clientType, req.Model); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleRemoveClientConfig(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	if err := removeClientConfig(clientType); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleBackupClientConfigFile(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	if err := backupClientConfigFile(clientType); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleHasClientConfigBackup(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	has := hasClientConfigBackup(clientType)
	writeJSON(w, http.StatusOK, map[string]bool{"has_backup": has})
}

func handleRestoreClientConfigFile(w http.ResponseWriter, r *http.Request) {
	clientType := r.PathValue("type")
	if err := restoreClientConfigFile(clientType); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Models
// ============================================================

func handleListQoderModels(w http.ResponseWriter, r *http.Request) {
	if headlessBridge == nil {
		writeError(w, http.StatusServiceUnavailable, "bridge not running")
		return
	}
	models, err := headlessBridge.ListAvailableModels()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

// ============================================================
// Logs
// ============================================================

func handleGetLogsSince(w http.ResponseWriter, r *http.Request) {
	afterSeq := intQuery(r, "after_seq", 0)
	limit := intQuery(r, "limit", 100)
	page := logger.GetLogsSince(afterSeq, limit)
	writeJSON(w, http.StatusOK, page)
}

func handleClearLogs(w http.ResponseWriter, r *http.Request) {
	logger.Clear()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ============================================================
// Update
// ============================================================

func handleGetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": updater.Version})
}

func handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := updater.Check()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "web mode does not support in-place desktop updates; update the deployed binary or container image instead")
}

// ============================================================
// Cleanup
// ============================================================

func handleCleanupAllData(w http.ResponseWriter, r *http.Request) {
	// Stop bridge
	headlessBridge = nil

	// Remove client configs
	for _, ct := range []string{"claude", "codex", "gemini"} {
		_ = removeClientConfig(ct)
	}

	// Remove account secrets
	accounts, _ := account.List()
	for _, acct := range accounts {
		_ = account.DeleteSecret(acct.ID)
	}

	// Remove data dir
	home, err := dataDir()
	if err == nil {
		_ = removeAll(home + "/.qccg")
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
