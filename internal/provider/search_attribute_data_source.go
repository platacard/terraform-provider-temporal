package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

// Ensures that SearchAttributeDataSource fully satisfies the datasource.DataSource and
// datasource.DataSourceWithConfigure interfaces.
var (
	_ datasource.DataSource              = &SearchAttributeDataSource{}
	_ datasource.DataSourceWithConfigure = &SearchAttributeDataSource{}
)

// NewSearchAttributeDataSource returns a new instance of the SearchAttributeDataSource.
func NewSearchAttributeDataSource() datasource.DataSource {
	return &SearchAttributeDataSource{}
}

// SearchAttributeDataSource implements the Terraform data source interface for Temporal SearchAttributes.
type SearchAttributeDataSource struct {
	client operatorservice.OperatorServiceClient
}

// SearchAttributeDataSourceModel defines the structure for the data source's configuration and read data.
type SearchAttributeDataSourceModel struct {
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Namespace types.String `tfsdk:"namespace"`
}

// Metadata sets the metadata for the Temporal SearchAttribute data source, specifically the type name.
func (d *SearchAttributeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_search_attribute"
}

// Schema defines the schema for the Temporal SearchAttribute data source.
func (d *SearchAttributeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Temporal Search Attribute data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Search Attribute Name",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Search Attribute Indexed Value Type, which defines the type of data stored in the attribute",
				Computed:            true,
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Namespace with which the Search Attribute is associated",
				Required:            true,
			},
		},
	}
}

// Configure sets up the SearchAttribute data source configuration.
func (d *SearchAttributeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal SearchAttribute DataSource")

	// Prevent panic if the provider has not been configured yet.
	if req.ProviderData == nil {
		return
	}

	connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	client := operatorservice.NewOperatorServiceClient(connection)
	d.client = client

	tflog.Info(ctx, "Configured Temporal Search Attribute client", map[string]any{"success": true})
}

// Read fetches data from a Temporal SearchAttribute and sets it in the Terraform state.
func (d *SearchAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Info(ctx, "Reading Temporal Search Attribute")

	var name, namespace string

	// Get the 'name' and 'namespace' attributes from the configuration
	diags := req.Config.GetAttribute(ctx, path.Root("name"), &name)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = req.Config.GetAttribute(ctx, path.Root("namespace"), &namespace)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := &operatorservice.ListSearchAttributesRequest{
		Namespace: namespace,
	}

	// Calling API for existing attribute details
	searchAttributes, err := d.client.ListSearchAttributes(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError("API Client Error", fmt.Sprintf("Unable to read SearchAttribute: %s", err))
		return
	}

	var attributeType enums.IndexedValueType
	var found bool
	if attributeType, found = searchAttributes.GetCustomAttributes()[name]; !found {
		if attributeType, found = searchAttributes.GetSystemAttributes()[name]; !found {
			resp.Diagnostics.AddError("Not Found", fmt.Sprintf("SearchAttribute '%s' not found in namespace '%s'", name, namespace))
			return
		}
	}

	// Prepare the data to be set in the Terraform state
	data := &SearchAttributeDataSourceModel{
		Name:      types.StringValue(name),
		Type:      types.StringValue(attributeType.String()),
		Namespace: types.StringValue(namespace),
	}

	// Save the fetched data into Terraform state
	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "SearchAttribute data source read successfully", map[string]any{"name": name, "type": attributeType.String(), "namespace": namespace})
}
