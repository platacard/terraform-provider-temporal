package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &NamespaceDataSource{}

func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// NamespaceDataSource defines the data source implementation.
type NamespaceDataSource struct{}

// NamespaceDataSourceModel describes the data source data model.
type NamespaceDataSourceModel struct {
	Name                    types.String `tfsdk:"name"`
	Id                      types.String `tfsdk:"id"`
	Description             types.String `tfsdk:"description"`
	OwnerEmail              types.String `tfsdk:"owner_email"`
	State                   types.String `tfsdk:"state"`
	ActiveClusterName       types.String `tfsdk:"active_cluster_name"`
	Clusters                types.List   `tfsdk:"clusters"`
	HistoryArchivalState    types.String `tfsdk:"history_archival_state"`
	VisibilityArchivalState types.String `tfsdk:"visibility_archival_state"`
	IsGlobalNamespace       types.String `tfsdk:"is_global_namespace"`
	FailoverVersion         types.Number `tfsdk:"failover_version"`
	FailoverHistory         types.List   `tfsdk:"failover_history"`
}

func (d *NamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (d *NamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Namespace data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Namespace name",
				Optional:            false,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Namespace identifier",
				Computed:            true,
			},
		},
	}
}

func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
}

func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NamespaceDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := d.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace, got error: %s", err))
	//     return
	// }

	// For the purposes of this Namespace code, hardcoding a response value to
	// save into the Terraform state.
	data.Id = types.StringValue("Namespace-id")

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
