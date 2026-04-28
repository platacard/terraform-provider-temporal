package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	grpcInsec "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
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

func TestCreateAuthenticatedClient_LazyTokenFetch(t *testing.T) {
	calls := 0
	srv := newMockTokenServer(t, &calls, 3600)
	defer srv.Close()

	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		srv.URL+"/token",
		"test-audience",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient returned error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if calls != 0 {
		t.Errorf("expected 0 token fetches at client creation, got %d — TokenSource must be lazy", calls)
	}
}

func TestCreateAuthenticatedClient_TokenServerUnreachable(t *testing.T) {
	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		"http://127.0.0.1:0/token",
		"test-audience",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient must not fail at construction when token server is unreachable: %v", err)
	}
	defer func() { _ = conn.Close() }()
}

func TestCreateAuthenticatedClient_RefreshesExpiredToken(t *testing.T) {
	calls := 0
	srv := newMockTokenServer(t, &calls, 0)
	defer srv.Close()

	conn, err := CreateAuthenticatedClient(
		"localhost:9999",
		"client-id",
		"client-secret",
		srv.URL+"/token",
		"test-audience",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient returned error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if calls != 0 {
		t.Errorf("expected 0 calls at construction, got %d", calls)
	}
}

// Auth0 and other spec-compliant servers require `audience` as a form param, not a scope.
func TestCreateAuthenticatedClient_PassesAudienceInTokenRequest(t *testing.T) {
	const wantAudience = "https://temporal.local"
	wantScopes := []string{"scope-a", "scope-b"}

	var (
		mu              sync.Mutex
		gotAudience     string
		gotScope        string
		tokenReqHandled = make(chan struct{}, 1)
	)
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		gotAudience = r.PostFormValue("audience")
		gotScope = r.PostFormValue("scope")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockTokenResponse{
			AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
		})
		select {
		case tokenReqHandled <- struct{}{}:
		default:
		}
	}))
	defer tokenSrv.Close()

	// Stub server so Invoke triggers the interceptor's token fetch; the call itself failing is fine.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = lis.Close() }()
	grpcSrv := grpc.NewServer()
	go func() { _ = grpcSrv.Serve(lis) }()
	defer grpcSrv.Stop()

	conn, err := CreateAuthenticatedClient(
		lis.Addr().String(),
		"client-id",
		"client-secret",
		tokenSrv.URL+"/token",
		wantAudience,
		wantScopes,
		grpcInsec.NewCredentials(),
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = conn.Invoke(ctx, "/no.svc/Method", &emptypb.Empty{}, &emptypb.Empty{})

	select {
	case <-tokenReqHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("token endpoint was never hit")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotAudience != wantAudience {
		t.Errorf("audience form param: got %q, want %q", gotAudience, wantAudience)
	}
	// Scopes are space-joined per RFC 6749 §3.3.
	if gotScope != "scope-a scope-b" {
		t.Errorf("scope form param: got %q, want %q", gotScope, "scope-a scope-b")
	}
}

// Audience must not leak into the scope= form field even when both are set.
func TestCreateGRPCClient_DoesNotReflectAudienceIntoScopes(t *testing.T) {
	const (
		wantAudience = "https://api.example/"
		wantScope    = "openid"
	)
	var (
		mu       sync.Mutex
		gotForm  url.Values
		captured = make(chan struct{}, 1)
	)
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		gotForm = r.PostForm
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockTokenResponse{
			AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600,
		})
		select {
		case captured <- struct{}{}:
		default:
		}
	}))
	defer tokenSrv.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = lis.Close() }()
	grpcSrv := grpc.NewServer()
	go func() { _ = grpcSrv.Serve(lis) }()
	defer grpcSrv.Stop()

	conn, err := CreateGRPCClient(
		"client-id", "client-secret",
		tokenSrv.URL+"/token",
		wantAudience,
		[]string{wantScope},
		lis.Addr().String(),
		true,
		false,
		"", "", "", "",
	)
	if err != nil {
		t.Fatalf("CreateGRPCClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = conn.Invoke(ctx, "/no.svc/Method", &emptypb.Empty{}, &emptypb.Empty{})

	select {
	case <-captured:
	case <-time.After(2 * time.Second):
		t.Fatal("token endpoint was never hit")
	}

	mu.Lock()
	defer mu.Unlock()
	if got := gotForm.Get("audience"); got != wantAudience {
		t.Errorf("audience form param: got %q, want %q", got, wantAudience)
	}
	if got := gotForm.Get("scope"); got != wantScope {
		t.Errorf("scope form param: got %q, want %q", got, wantScope)
	}
}
