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
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ provider.Provider = &TemporalProvider{}

// TemporalProvider defines the provider implementation.
type TemporalProvider struct {
	version string
}

// TemporalProviderModel describes the provider data model.
type temporalProviderModel struct {
	Host types.String `tfsdk:"host"`
	Port types.String `tfsdk:"port"`
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
		},
	}
}

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

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	host := os.Getenv("TEMPORAL_HOST")
	port := os.Getenv("TEMPORAL_PORT")

	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}

	if !config.Port.IsNull() {
		port = config.Port.ValueString()
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

	if resp.Diagnostics.HasError() {
		return
	}

	// Create a new Temporal client using the configuration values
	// jwtCreds := strings.Join([]string{"Bearer", token}, " ")
	ctx = tflog.SetField(ctx, "temporal_host", host)
	ctx = tflog.SetField(ctx, "temporal_port", port)

	tflog.Debug(ctx, "Creating Temporal client")

	endpoint := strings.Join([]string{host, port}, ":")
	client, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Temporal API Client",
			"An unexpected error occurred when creating the Temporal API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Temporal Client Error: "+err.Error(),
		)
		return
	}
	// connection, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")), grpcMetadata.New(map[string]string{"authorization": jwtCreds}))

	// Make the Temporal client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client

	tflog.Info(ctx, "Configured Temporal client", map[string]any{"success": true})
}

func (p *TemporalProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNamespaceResource,
	}
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
