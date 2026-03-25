package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	grpcInsec "google.golang.org/grpc/credentials/insecure"
)

type mockTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func newMockTokenServer(t *testing.T, calls *int, expiresIn int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "application/json")
		resp := mockTokenResponse{
			AccessToken: fmt.Sprintf("token-%d", *calls),
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestCreateAuthenticatedClient_LazyTokenFetch verifies that CreateAuthenticatedClient
// does not fetch an OAuth2 token at construction time. The token must only be
// fetched when an actual gRPC call is made
func TestCreateAuthenticatedClient_LazyTokenFetch(t *testing.T) {
	calls := 0
	srv := newMockTokenServer(t, &calls, 3600)
	defer srv.Close()

	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		srv.URL+"/token",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient returned error: %v", err)
	}
	defer conn.Close()

	if calls != 0 {
		t.Errorf("expected 0 token fetches at client creation, got %d — TokenSource must be lazy", calls)
	}
}

// TestCreateAuthenticatedClient_TokenServerUnreachable verifies that
// CreateAuthenticatedClient does not fail when the token server is unreachable.
// Before the fix, GetToken() was called eagerly at startup and would have
// returned an error here
func TestCreateAuthenticatedClient_TokenServerUnreachable(t *testing.T) {
	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		"http://127.0.0.1:0/token",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient must not fail at construction when token server is unreachable: %v", err)
	}
	defer conn.Close()
}

// TestCreateAuthenticatedClient_RefreshesExpiredToken verifies that the token
// server is not called at construction even when tokens would expire immediately
// (expires_in=0). The old code fetched the token once at startup; the new code
// uses a TokenSource that refreshes lazily on each gRPC call
func TestCreateAuthenticatedClient_RefreshesExpiredToken(t *testing.T) {
	calls := 0
	srv := newMockTokenServer(t, &calls, 0) // expires_in=0 = token expires immediately
	defer srv.Close()

	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		srv.URL+"/token",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient returned error: %v", err)
	}
	defer conn.Close()

	if calls != 0 {
		t.Errorf("expected 0 calls at construction, got %d", calls)
	}
}
