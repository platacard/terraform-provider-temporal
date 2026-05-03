// Package provider implements the Terraform provider for Temporal.
// It facilitates the management of Temporal resources like namespaces.
// The provider supports configuration for connection to a Self-Hosted Temporal server.

package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc"
	grpcCreds "google.golang.org/grpc/credentials"
	grpcInsec "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// TemporalProvider implements the provider interface for Temporal.
// It is used to configure and manage Temporal resources.
var _ provider.Provider = &TemporalProvider{}

// TemporalProvider defines the structure for the Temporal provider.
type TemporalProvider struct {
	version string
}

// temporalProviderModel defines the configuration structure for the Temporal provider.
// It includes the host and port for connecting to the Temporal server.
type temporalProviderModel struct {
	Host         types.String `tfsdk:"host"`
	Port         types.String `tfsdk:"port"`
	ClientSecret types.String `tfsdk:"client_secret"`
	ClientID     types.String `tfsdk:"client_id"`
	TokenURL     types.String `tfsdk:"token_url"`
	Audience     types.String `tfsdk:"audience"`
	Scopes       types.List   `tfsdk:"scopes"`
	Insecure     types.Bool   `tfsdk:"insecure"`
	TLS          types.Object `tfsdk:"tls"`
	GrpcMetadata types.Map    `tfsdk:"grpc_metadata"`
}

// Metadata assigns the provider's name and version.
func (p *TemporalProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "temporal"
	resp.Version = p.version
}

// Schema defines the configuration schema for the provider.
func (p *TemporalProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Blocks: map[string]schema.Block{
			"tls": schema.SingleNestedBlock{
				Description: "TLS Configuration for the Temporal server",
				Attributes: map[string]schema.Attribute{
					"cert": schema.StringAttribute{
						Optional:    true,
						Description: "Client certificate PEM",
					},
					"key": schema.StringAttribute{
						Optional:    true,
						Description: "Private key PEM",
					},
					"ca": schema.StringAttribute{
						Optional:    true,
						Description: "CA certificates",
					},
					"cert_reload_time": schema.Int64Attribute{
						Optional:    true,
						Description: "Certificate reload time",
					},
					"server_name": schema.StringAttribute{
						Optional:    true,
						Description: "Used to verify the hostname and included in handshake",
					},
				},
			},
		},
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "The Temporal server host.",
				Optional:    true,
			},
			"port": schema.StringAttribute{
				Description: "The Temporal server port.",
				Optional:    true,
			},
			"token_url": schema.StringAttribute{
				Optional:    true,
				Description: "Oauth2 server URL to fetch token from",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_id")),
					stringvalidator.AlsoRequires(path.MatchRoot("client_secret")),
				},
			},
			"client_id": schema.StringAttribute{
				Optional:    true,
				Description: "The OAuth2 Client ID for API operations.",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_secret")),
					stringvalidator.AlsoRequires(path.MatchRoot("token_url")),
				},
			},
			"client_secret": schema.StringAttribute{
				Optional:    true,
				Description: "The OAuth2 Client Secret for API operations.",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("token_url")),
					stringvalidator.AlsoRequires(path.MatchRoot("client_id")),
				},
			},
			"audience": schema.StringAttribute{
				Optional:    true,
				Description: "Audience of the token.",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_id")),
					stringvalidator.AlsoRequires(path.MatchRoot("client_secret")),
					stringvalidator.AlsoRequires(path.MatchRoot("token_url")),
				},
			},
			"scopes": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: `OAuth2 scopes requested when fetching a token. Defaults to ["openid", "profile", "email"].`,
			},
			"insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Use insecure connection",
			},
			"grpc_metadata": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Sensitive:   true,
				Description: "Static gRPC metadata appended to every outgoing call (case-insensitive keys, string values). Useful for reverse-proxy authentication that lives at the HTTP/2 header layer rather than the OAuth Bearer layer — for example Cloudflare Access service tokens (`cf-access-client-id` / `cf-access-client-secret`), Tailscale Funnel, or AWS WAF custom rules. Marked sensitive because credentials are common values; the map is also overrideable via `TEMPORAL_GRPC_METADATA` (comma-separated `k=v` pairs).",
			},
		},
	}
}

// Configure sets up the provider with the given configuration.
// It validates the config and initializes the Temporal client connection.
func (p *TemporalProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal client")

	// Retrieve provider data from configuration
	var config temporalProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.
	if config.Host.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Unknown Tmeporal Frontend Host",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Temporal API host. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_HOST environment variable.",
		)
	}

	if config.Port.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("port"),
			"Unknown Temporal Frontend Port",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Temporal API port. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_PORT environment variable.",
		)
	}

	if config.ClientID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_id"),
			"Unknown Temporal Client ID",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Temporal API Client ID. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_CLIENT_ID environment variable.",
		)
	}
	if config.ClientSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("client_secret"),
			"Unknown Temporal Client Secret",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Temporal API Client Secret. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_CLIENT_SECRET environment variable.",
		)
	}
	if config.TokenURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_url"),
			"Unknown Oauth2 Token URL",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Oauth2 Token URL. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_TOKEN_URL environment variable.",
		)
	}
	if config.Audience.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("audience"),
			"Unknown Audience",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Oauth2 Client Audience. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_AUDIENCE environment variable.",
		)
	}
	if config.Scopes.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("scopes"),
			"Unknown Scopes",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the OAuth2 Scopes. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_SCOPES environment variable.",
		)
	}
	if config.Insecure.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure"),
			"Unknown Insecure",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Insecure option. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_INSECURE environment variable.",
		)
	}
	if config.GrpcMetadata.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("grpc_metadata"),
			"Unknown gRPC Metadata",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the gRPC metadata map. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_GRPC_METADATA environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	host := os.Getenv("TEMPORAL_HOST")
	port := os.Getenv("TEMPORAL_PORT")
	tokenURL := os.Getenv("TEMPORAL_TOKEN_URL")
	clientID := os.Getenv("TEMPORAL_CLIENT_ID")
	clientSecret := os.Getenv("TEMPORAL_CLIENT_SECRET")
	audience := os.Getenv("TEMPORAL_AUDIENCE")
	scopes := parseScopesEnv(os.Getenv("TEMPORAL_SCOPES"))
	grpcMetadata := parseGrpcMetadataEnv(os.Getenv("TEMPORAL_GRPC_METADATA"))
	insecure, err := getBoolEnv("TEMPORAL_INSECURE")
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure"),
			"Unknown Insecure",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Insecure option. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_INSECURE environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}
	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.Port.IsNull() {
		port = config.Port.ValueString()
	}

	if !config.TokenURL.IsNull() {
		tokenURL = config.TokenURL.ValueString()
	}
	if !config.ClientID.IsNull() {
		clientID = config.ClientID.ValueString()
	}
	if !config.ClientSecret.IsNull() {
		clientSecret = config.ClientSecret.ValueString()
	}
	if !config.Audience.IsNull() {
		audience = config.Audience.ValueString()
	}
	if !config.Scopes.IsNull() {
		scopes = nil
		diags := config.Scopes.ElementsAs(ctx, &scopes, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	if !config.GrpcMetadata.IsNull() {
		grpcMetadata = map[string]string{}
		diags := config.GrpcMetadata.ElementsAs(ctx, &grpcMetadata, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !config.Insecure.IsNull() {
		insecure = config.Insecure.ValueBool()
	}

	var (
		certString string
		keyString  string
		caCerts    string
		serverName string
	)

	var useTLS = false

	if !config.TLS.IsNull() {
		useTLS = true

		tlsAttributes := config.TLS.Attributes()

		certString = normalizeCert(tlsAttributes["cert"].String())
		keyString = normalizeCert(tlsAttributes["key"].String())
		caCerts = normalizeCert(tlsAttributes["ca"].String())

		if !tlsAttributes["server_name"].IsNull() {
			serverName = stripQuotes(tlsAttributes["server_name"].String())
		}
	}

	// If host and port not set use defaults
	if host == "" {
		host = "127.0.0.1"
	}

	if port == "" {
		port = "7233"
	}

	// Create a new Temporal client using the configuration values
	ctx = tflog.SetField(ctx, "temporal_host", host)
	ctx = tflog.SetField(ctx, "temporal_port", port)
	endpoint := strings.Join([]string{host, port}, ":")

	tflog.Debug(ctx, "Creating Temporal client")
	tflog.Debug(ctx, "Use TLS? "+strconv.FormatBool(useTLS))
	client, err := CreateGRPCClient(clientID, clientSecret, tokenURL, audience, scopes, endpoint, insecure, useTLS, certString, keyString, caCerts, serverName, grpcMetadata)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Temporal API Client",
			"An unexpected error occurred when creating the Temporal API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Temporal Client Error: "+err.Error(),
		)
		return
	}

	// Make the Temporal client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "Configured Temporal client", map[string]any{"success": true})
}

// Resources returns a list of resource types managed by this provider.
func (p *TemporalProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNamespaceResource,
		NewSearchAttributeResource,
		NewScheduleResource,
	}
}

// DataSources returns a list of data source types managed by this provider.
func (p *TemporalProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNamespaceDataSource,
		NewSearchAttributeDataSource,
	}
}

// New is a constructor for the TemporalProvider.
// It takes a version string and returns a new TemporalProvider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &TemporalProvider{
			version: version,
		}
	}
}

// metadataPairs flattens a metadata map into the alternating
// key,value,key,value... slice that metadata.AppendToOutgoingContext expects.
// Returns nil for empty input so callers can elide the interceptor entirely.
func metadataPairs(md map[string]string) []string {
	if len(md) == 0 {
		return nil
	}
	out := make([]string, 0, 2*len(md))
	for k, v := range md {
		out = append(out, k, v)
	}
	return out
}

// metadataUnaryInterceptor returns a UnaryClientInterceptor that appends the
// given metadata to every outgoing call's context, or nil if md is empty.
func metadataUnaryInterceptor(md map[string]string) grpc.UnaryClientInterceptor {
	pairs := metadataPairs(md)
	if pairs == nil {
		return nil
	}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// metadataStreamInterceptor is the streaming counterpart of
// metadataUnaryInterceptor. Temporal admin operations are unary today, but
// future Temporal SDK versions may use streams (long polls); this keeps the
// metadata applied uniformly.
func metadataStreamInterceptor(md map[string]string) grpc.StreamClientInterceptor {
	pairs := metadataPairs(md)
	if pairs == nil {
		return nil
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// dialOptionsForMetadata returns gRPC dial options that inject the given
// metadata on every call (both unary and streaming). Returns nil slice for
// empty input.
func dialOptionsForMetadata(md map[string]string) []grpc.DialOption {
	var opts []grpc.DialOption
	if u := metadataUnaryInterceptor(md); u != nil {
		opts = append(opts, grpc.WithChainUnaryInterceptor(u))
	}
	if s := metadataStreamInterceptor(md); s != nil {
		opts = append(opts, grpc.WithChainStreamInterceptor(s))
	}
	return opts
}

// parseGrpcMetadataEnv parses the TEMPORAL_GRPC_METADATA env var. Format:
// comma-separated `key=value` pairs (e.g.
// "cf-access-client-id=abc,cf-access-client-secret=def"). Empty values and
// malformed pairs are silently dropped — provider config takes precedence
// anyway.
func parseGrpcMetadataEnv(s string) map[string]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.Index(p, "=")
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(p[:eq])
		v := strings.TrimSpace(p[eq+1:])
		if k != "" && v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CreateAuthenticatedClient creates a gRPC client with OAuth authentication.
// It uses a TokenSource so the token is automatically refreshed when it expires.
// Any extraMetadata is appended to every outgoing call alongside the OAuth
// Authorization header.
func CreateAuthenticatedClient(endpoint string, clientID, clientSecret, tokenURL, audience string, scopes []string, credentials grpcCreds.TransportCredentials, extraMetadata map[string]string) (*grpc.ClientConn, error) {
	cfg := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       scopes,
	}
	if audience != "" {
		cfg.EndpointParams = url.Values{"audience": {audience}}
	}
	ts := cfg.TokenSource(context.Background())

	authInterceptor := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		token, err := ts.Token()
		if err != nil {
			return fmt.Errorf("failed to retrieve OAuth token: %w", err)
		}
		newCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token.AccessToken)
		return invoker(newCtx, method, req, reply, cc, opts...)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials),
		grpc.WithChainUnaryInterceptor(authInterceptor),
	}
	dialOpts = append(dialOpts, dialOptionsForMetadata(extraMetadata)...)
	return grpc.NewClient(endpoint, dialOpts...)
}

// CreateSecureClient creates a gRPC client using mTLS without OAuth authentication.
// Any extraMetadata is appended to every outgoing call.
func CreateSecureClient(endpoint string, credentials grpcCreds.TransportCredentials, extraMetadata map[string]string) (*grpc.ClientConn, error) {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(credentials)}
	dialOpts = append(dialOpts, dialOptionsForMetadata(extraMetadata)...)
	return grpc.NewClient(endpoint, dialOpts...)
}

// CreateInsecureClient creates a gRPC client without any authentication.
// Any extraMetadata is appended to every outgoing call (useful when an
// upstream reverse proxy provides the auth, e.g. Cloudflare Access service
// tokens, even though this gRPC hop is plaintext).
func CreateInsecureClient(endpoint string, credentials grpcCreds.TransportCredentials, extraMetadata map[string]string) (*grpc.ClientConn, error) {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(credentials)}
	dialOpts = append(dialOpts, dialOptionsForMetadata(extraMetadata)...)
	return grpc.NewClient(endpoint, dialOpts...)
}

// CreateGRPCClient decides which gRPC client to create based on clientID.
func CreateGRPCClient(clientID, clientSecret, tokenURL, audience string, scopes []string, endpoint string, insecure bool, useTLS bool, certString string, keyString string, caCerts string, serverName string, grpcMetadata map[string]string) (*grpc.ClientConn, error) {
	var credentials grpcCreds.TransportCredentials

	switch insecure {
	case true:
		credentials = grpcInsec.NewCredentials()
	case false:
		switch useTLS {
		case true:
			cert, err := tls.X509KeyPair([]byte(certString), []byte(keyString))

			if err != nil {
				return nil, err
			}

			config := &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      getCA([]byte(caCerts)),
			}

			if len(serverName) > 0 {
				config.ServerName = serverName
			}

			credentials = grpcCreds.NewTLS(config)
		case false:
			config := &tls.Config{}
			credentials = grpcCreds.NewTLS(config)
		}
	}

	if clientID != "" {
		return CreateAuthenticatedClient(endpoint, clientID, clientSecret, tokenURL, audience, scopes, credentials, grpcMetadata)
	} else if useTLS {
		return CreateSecureClient(endpoint, credentials, grpcMetadata)
	}

	return CreateInsecureClient(endpoint, credentials, grpcMetadata)
}

// Function to get CA certificates.
func getCA(caCerts []byte) *x509.CertPool {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCerts)
	return caCertPool
}

// parseScopesEnv splits a comma-separated env var value into trimmed scope strings.
func parseScopesEnv(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	scopes := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	if len(scopes) == 0 {
		return nil
	}
	return scopes
}

func getBoolEnv(key string) (result bool, err error) {
	val, exist := os.LookupEnv(key)
	if !exist {
		return false, err
	}
	result, err = strconv.ParseBool(val)
	if err != nil {
		return false, err
	}
	return result, err
}

// Helper function to strip quotes and remove line return escaping from cert.
func normalizeCert(value string) string {
	return strings.ReplaceAll(stripQuotes(value), "\\n", "\n")
}

// Helper function to strip quotes from string.
func stripQuotes(value string) string {
	return strings.TrimPrefix(strings.TrimSuffix(value, "\""), "\"")
}
