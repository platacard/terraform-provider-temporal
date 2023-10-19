package provider

import (
	"context"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	operatorservice "go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcMetadata "google.golang.org/grpc/metadata"
)

var _ provider.Provider = &TemporalProvider{}

// TemporalProvider defines the provider implementation.
type TemporalProvider struct {
	version string
}

// TemporalProviderModel describes the provider data model.
type temporalProviderModel struct {
	Host  types.String `tfsdk:"host"`
	Port  types.String `tfsdk:"port"`
	Token types.String `tfsdk:"token"`
}

func (p *TemporalProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "temporal"
	resp.Version = p.version
}

func (p *TemporalProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Optional: true,
			},
			"port": schema.StringAttribute{
				Optional: true,
			},
			"token": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}

func (p *TemporalProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
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

	if config.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown Temporal Auth Token",
			"The provider cannot create the Temporal API client as there is an unknown configuration value for the Temporal Auth Token. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the TEMPORAL_TOKEN environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	host := os.Getenv("TEMPORAL_HOST")
	port := os.Getenv("TEMPORAL_PORT")
	token := os.Getenv("TEMPORAL_TOKEN")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.Port.IsNull() {
		port = config.Port.ValueString()
	}

	if !config.Token.IsNull() {
		token = config.Token.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if host == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("host"),
			"Missing Temporal Frontend Host",
			"The provider cannot create the Temporal API client as there is a missing or empty value for the Temporal Frontend host. "+
				"Set the host value in the configuration or use the TEMPORAL_HOST environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if port == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("port"),
			"Missing Temporal Frontend Port",
			"The provider cannot create the Temporal API client as there is a missing or empty value for the Temporal Frontend port. "+
				"Set the username value in the configuration or use the TEMPORAL_PORT environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Missing Temporal Auth Token",
			"The provider cannot create the Temporal API client as there is a missing or empty value for the Temporal Auth Token. "+
				"Set the password value in the configuration or use the TEMPORAL_TOKEN environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Create a new Temporal client using the configuration values
	jwtCreds := strings.Join([]string{"Bearer", token}, " ")
	endpoint := strings.Join([]string{host, port}, ":")
	connection, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")), grpcMetadata.New(map[string]string{"authorization": jwtCreds}))

	client, err := operatorservice.NewOperatorServiceClient(connection)
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
}

func (p *TemporalProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}

func (p *TemporalProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNamespaceDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &TemporalProvider{
			version: version,
		}
	}
}
