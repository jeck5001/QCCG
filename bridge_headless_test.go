//go:build headless

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"qccg/account"
)

func TestHeadlessManagementAPIIsMountedWithFullPrefix(t *testing.T) {
	templateBase := map[string]interface{}{}
	root := newHeadlessMux("", account.RegionGlobal, templateBase, nil, 8963, "qccg")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rr := httptest.NewRecorder()

	root.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/status returned %d, want %d", rr.Code, http.StatusOK)
	}

	var body statusResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if body.Port != 8963 {
		t.Fatalf("port = %d, want 8963", body.Port)
	}
	if body.Region != "global" {
		t.Fatalf("region = %q, want global", body.Region)
	}
}

func TestResolveHeadlessTokenAllowsWebOnlyStartup(t *testing.T) {
	t.Setenv("QODER_PAT", "")
	token, err := resolveHeadlessToken(context.Background())
	if err != nil {
		t.Fatalf("resolveHeadlessToken returned error: %v", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want empty token for web-only startup", token)
	}
}
