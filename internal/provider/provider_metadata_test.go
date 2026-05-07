package provider

import (
	"context"
	"net"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	grpcInsec "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// captureMD installs an UnknownServiceHandler that records the incoming
// metadata of the first request, then signals via done. UnknownServiceHandler
// runs even when no service is registered for the called method (the
// alternative, UnaryInterceptor, only fires for registered services and
// returns Unimplemented before reaching the interceptor for unknown calls).
func captureMD(t *testing.T, lis net.Listener) (*metadata.MD, chan struct{}) {
	t.Helper()
	got := &metadata.MD{}
	var mu sync.Mutex
	done := make(chan struct{}, 1)

	srv := grpc.NewServer(grpc.UnknownServiceHandler(
		func(srv interface{}, stream grpc.ServerStream) error {
			md, _ := metadata.FromIncomingContext(stream.Context())
			mu.Lock()
			*got = md
			mu.Unlock()
			select {
			case done <- struct{}{}:
			default:
			}
			// Return success so the client doesn't see Unimplemented.
			return nil
		},
	))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return got, done
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	return lis
}

func mdValues(md metadata.MD, key string) []string {
	v := md.Get(key)
	sort.Strings(v)
	return v
}

func TestMetadata_InsecureClient_AppendsHeaders(t *testing.T) {
	lis := mustListen(t)
	got, done := captureMD(t, lis)

	conn, err := CreateInsecureClient(
		lis.Addr().String(),
		grpcInsec.NewCredentials(),
		map[string]string{
			"cf-access-client-id":     "abc.access",
			"cf-access-client-secret": "shh",
		},
	)
	if err != nil {
		t.Fatalf("CreateInsecureClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// UnknownServiceHandler returns success but without a typed response, so
	// proto unmarshal fails on the client side. We don't care about the
	// response — only that the request reached the server with our metadata.
	_ = conn.Invoke(ctx, "/no.svc/Method", &emptypb.Empty{}, &emptypb.Empty{})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the call")
	}

	if v := mdValues(*got, "cf-access-client-id"); !reflect.DeepEqual(v, []string{"abc.access"}) {
		t.Errorf("cf-access-client-id = %v, want [abc.access]", v)
	}
	if v := mdValues(*got, "cf-access-client-secret"); !reflect.DeepEqual(v, []string{"shh"}) {
		t.Errorf("cf-access-client-secret = %v, want [shh]", v)
	}
}

func TestMetadata_NilMap_OmitsInterceptor(t *testing.T) {
	// The whole point of returning nil from metadataUnaryInterceptor when md
	// is empty is to avoid a no-op interceptor in the dial chain. Verify
	// that dialOptionsForMetadata returns an empty slice for nil/empty maps.
	for name, in := range map[string]map[string]string{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			if got := dialOptionsForMetadata(in); len(got) != 0 {
				t.Errorf("dialOptionsForMetadata(%v) = %d opts, want 0", in, len(got))
			}
		})
	}
}

func TestParseGrpcMetadataEnv(t *testing.T) {
	cases := map[string]struct {
		in   string
		want map[string]string
	}{
		"empty":          {"", nil},
		"single":         {"k=v", map[string]string{"k": "v"}},
		"two":            {"a=1,b=2", map[string]string{"a": "1", "b": "2"}},
		"surrounding ws": {"  k = v  ,  m = n  ", map[string]string{"k": "v", "m": "n"}},
		"missing eq":     {"a=1,nope,b=2", map[string]string{"a": "1", "b": "2"}},
		"empty key":      {"=v,b=2", map[string]string{"b": "2"}},
		"empty value":    {"a=,b=2", map[string]string{"b": "2"}},
		"all garbage":    {",,,", nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := parseGrpcMetadataEnv(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseGrpcMetadataEnv(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestMetadata_AuthenticatedClient_AppendsBothBearerAndExtras(t *testing.T) {
	// Mirror the OAuth test pattern: a mock token endpoint and a stub gRPC
	// server that captures the incoming metadata. Verify the OAuth Bearer
	// header AND the extra metadata both arrive.
	lis := mustListen(t)
	got, done := captureMD(t, lis)

	calls := 0
	tokSrv := newMockTokenServer(t, &calls, 3600)
	defer tokSrv.Close()

	conn, err := CreateAuthenticatedClient(
		lis.Addr().String(),
		"client-id",
		"client-secret",
		tokSrv.URL+"/token",
		"audience",
		[]string{"scope"},
		grpcInsec.NewCredentials(),
		map[string]string{"cf-access-client-id": "abc.access"},
	)
	if err != nil {
		t.Fatalf("CreateAuthenticatedClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// UnknownServiceHandler returns success but without a typed response, so
	// proto unmarshal fails on the client side. We don't care about the
	// response — only that the request reached the server with our metadata.
	_ = conn.Invoke(ctx, "/no.svc/Method", &emptypb.Empty{}, &emptypb.Empty{})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the call")
	}

	if v := (*got).Get("authorization"); len(v) != 1 || v[0] == "" {
		t.Errorf("authorization header missing, got %v", v)
	}
	if v := mdValues(*got, "cf-access-client-id"); !reflect.DeepEqual(v, []string{"abc.access"}) {
		t.Errorf("cf-access-client-id = %v, want [abc.access]", v)
	}
}
