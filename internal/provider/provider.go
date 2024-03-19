// Package provider implements the Terraform provider for Temporal.
// It facilitates the management of Temporal resources like namespaces.
// The provider supports configuration for connection to a Self-Hosted Temporal server.

package provider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
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
	"golang.org/x/oauth2"
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
	Insecure     types.Bool   `tfsdk:"insecure"`
	TLS          types.Object `tfsdk:"tls"`
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
			"insecure": schema.BoolAttribute{
				Optional:    true,
				Description: "Use insecure connection",
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
	if config.Insecure.IsUnknown() {
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

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	host := os.Getenv("TEMPORAL_HOST")
	port := os.Getenv("TEMPORAL_PORT")
	tokenURL := os.Getenv("TEMPORAL_TOKEN_URL")
	clientID := os.Getenv("TEMPORAL_CLIENT_ID")
	clientSecret := os.Getenv("TEMPORAL_CLIENT_SECRET")
	audience := os.Getenv("TEMPORAL_AUDIENCE")
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
	if !config.Insecure.IsNull() {
		insecure = config.Insecure.ValueBool()
	}

	var (
		certString string
		keyString  string
		caCerts    string
		serverName string
	)

	var useTLS bool = false

	if !config.TLS.IsNull() {
		useTLS = true

		tlsAttributes := config.TLS.Attributes()

		certString = normalizeCert(tlsAttributes["cert"].String())
		keyString = normalizeCert(tlsAttributes["key"].String())
		caCerts = normalizeCert(tlsAttributes["ca"].String())
		serverName = stripQuotes(tlsAttributes["server_name"].String())
	}

	// If host and port not set use defaults
	if host == "" {
		host = "127.0.0.1"
	}

	if port == "" {
		port = "7233"
	}

	if audience == "" {
		audience = "openid,profile,email"
	}

	// Create a new Temporal client using the configuration values
	ctx = tflog.SetField(ctx, "temporal_host", host)
	ctx = tflog.SetField(ctx, "temporal_port", port)
	endpoint := strings.Join([]string{host, port}, ":")

	tflog.Debug(ctx, "Creating Temporal client")
	tflog.Debug(ctx, "Use TLS? "+strconv.FormatBool(useTLS))
	client, err := CreateGRPCClient(clientID, clientSecret, tokenURL, audience, endpoint, insecure, useTLS, certString, keyString, caCerts, serverName)
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
	}
}

// DataSources returns a list of data source types managed by this provider.
func (p *TemporalProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNamespaceDataSource,
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

// GetToken retrieves an OAuth token using client credentials.
func GetToken(clientID, clientSecret, tokenURL, audience string) (*oauth2.Token, error) {
	clientCredentials := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       strings.Split(audience, ","),
	}

	token, err := clientCredentials.Token(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %v", err)
	}

	return token, nil
}

// CreateAuthenticatedClient creates a gRPC client with OAuth authentication.
func CreateAuthenticatedClient(endpoint string, token *oauth2.Token, credentials grpcCreds.TransportCredentials) (*grpc.ClientConn, error) {
	return grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials), grpc.WithUnaryInterceptor(
		func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			newCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token.AccessToken)
			return invoker(newCtx, method, req, reply, cc, opts...)
		},
	))
}

// CreateSecureClient creates a gRPC client using mTLS without OAuth authentication.
func CreateSecureClient(endpoint string, credentials grpcCreds.TransportCredentials) (*grpc.ClientConn, error) {
	return grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials))
}

// CreateInsecureClient creates a gRPC client without any authentication.
func CreateInsecureClient(endpoint string, credentials grpcCreds.TransportCredentials) (*grpc.ClientConn, error) {
	return grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials))
}

// CreateGRPCClient decides which gRPC client to create based on clientID.
func CreateGRPCClient(clientID, clientSecret, tokenURL, audience, endpoint string, insecure bool, useTLS bool, certString string, keyString string, caCerts string, serverName string) (*grpc.ClientConn, error) {
	var credentials grpcCreds.TransportCredentials

	switch insecure {
	case true:
		credentials = grpcInsec.NewCredentials()
	case false:
		switch useTLS {
		case true:
			// Parse the certificate from PEM format
			cert, err := getCertificate(certString)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %v", err)
			}

			// Parse the private key from PEM format
			key, err := getPrivateKey([]byte(keyString))
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key: %v", err)
			}

			// Create a tls.Certificate from the parsed certificate and private key
			tlsCert := tls.Certificate{
				Certificate: [][]byte{cert.Raw},
				PrivateKey:  key,
			}

			config := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				RootCAs:      getCA([]byte(caCerts)),
				ServerName:   serverName,
			}
			credentials = grpcCreds.NewTLS(config)
		case false:
			config := &tls.Config{}
			credentials = grpcCreds.NewTLS(config)
		}
	}

	if clientID != "" {
		token, err := GetToken(clientID, clientSecret, tokenURL, audience)
		if err != nil {
			return nil, err
		}

		return CreateAuthenticatedClient(endpoint, token, credentials)
	} else if useTLS {
		return CreateSecureClient(endpoint, credentials)
	}

	return CreateInsecureClient(endpoint, credentials)
}

// Function to parse the public key from PEM format.
func getCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))

	if block == nil {
		return nil, fmt.Errorf("failed to decode cert PEM. certPEM bytes:\n%v", []byte(certPEM))
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %v", err)
	}

	return cert, nil
}

// Function to parse the private key from PEM format.
func getPrivateKey(keyPEM []byte) (interface{}, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// If parsing as PKCS1 fails, try parsing as PKCS8
		pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
		return pkcs8Key, nil
	}

	return key, nil
}

// Function to get CA certificates.
func getCA(caCerts []byte) *x509.CertPool {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCerts)
	return caCertPool
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
	return strings.Replace(stripQuotes(value), "\\n", "\n", -1)
}

// Helper function to strip quotes from string.
func stripQuotes(value string) string {
	return strings.TrimPrefix(strings.TrimSuffix(value, "\""), "\"")
}
