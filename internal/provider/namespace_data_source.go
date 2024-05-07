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
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

// Ensures that NamespaceDataSource fully satisfies the datasource.DataSource and
// datasource.DataSourceWithConfigure interfaces.
var (
	_ datasource.DataSource              = &NamespaceDataSource{}
	_ datasource.DataSourceWithConfigure = &NamespaceDataSource{}
)

// NewNamespaceDataSource returns a new instance of the NamespaceDataSource.
func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// NamespaceDataSource implements the Terraform data source interface for Temporal namespaces.
type NamespaceDataSource struct {
	client workflowservice.WorkflowServiceClient
}

// NamespaceDataSourceModel defines the structure for the data source's configuration and read data.
type NamespaceDataSourceModel struct {
	Name                    types.String `tfsdk:"name"`
	Id                      types.String `tfsdk:"id"`
	Description             types.String `tfsdk:"description"`
	OwnerEmail              types.String `tfsdk:"owner_email"`
	Retention               types.Int64  `tfsdk:"retention"`
	ActiveClusterName       types.String `tfsdk:"active_cluster_name"`
	HistoryArchivalState    types.String `tfsdk:"history_archival_state"`
	HistoryArchivalUri      types.String `tfsdk:"history_archival_uri"`
	VisibilityArchivalState types.String `tfsdk:"visibility_archival_state"`
	VisibilityArchivalUri   types.String `tfsdk:"visibility_archival_uri"`
	IsGlobalNamespace       types.Bool   `tfsdk:"is_global_namespace"`
}

// Metadata sets the metadata for the Temporal namespace data source, specifically the type name.
func (d *NamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

// Schema defines the schema for the Temporal namespace data source.
func (d *NamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Namespace data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Namespace name",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Namespace identifier",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Namespace Description",
				Computed:            true,
			},
			"owner_email": schema.StringAttribute{
				MarkdownDescription: "Namespace Owner Email",
				Computed:            true,
			},
			"retention": schema.Int64Attribute{
				MarkdownDescription: "Workflow Execution retention",
				Computed:            true,
			},
			"active_cluster_name": schema.StringAttribute{
				MarkdownDescription: "Active Cluster Name",
				Computed:            true,
			},
			"history_archival_state": schema.StringAttribute{
				MarkdownDescription: "History Archival State",
				Computed:            true,
			},
			"history_archival_uri": schema.StringAttribute{
				MarkdownDescription: "History Archival URI",
				Computed:            true,
				Optional:            true,
			},
			"visibility_archival_state": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival State",
				Computed:            true,
			},
			"visibility_archival_uri": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival URI",
				Computed:            true,
				Optional:            true,
			},
			"is_global_namespace": schema.BoolAttribute{
				MarkdownDescription: "Namespace is Global",
				Computed:            true,
			},
		},
	}
}

// Configure sets up the namespace data source configuration.
func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal Namespace DataSource")

	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface), got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	client := workflowservice.NewWorkflowServiceClient(connection)
	d.client = client

	tflog.Info(ctx, "Configured Temporal Namespace client", map[string]any{"success": true})
}

// Read fetches data from a Temporal namespace and sets it in the Terraform state.
func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Info(ctx, "Reading Temporal Namespace Info")

	var name string
	diags := req.Config.GetAttribute(ctx, path.Root("name"), &name)

	resp.Diagnostics.Append(diags...)
	ns, err := d.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: name,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "read a data source")

	data := &NamespaceDataSourceModel{
		Name:                    types.StringValue(ns.NamespaceInfo.GetName()),
		Id:                      types.StringValue(ns.NamespaceInfo.GetId()),
		Description:             types.StringValue(ns.NamespaceInfo.GetDescription()),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.GetOwnerEmail()),
		Retention:               types.Int64Value(int64(ns.Config.WorkflowExecutionRetentionTtl.AsDuration().Hours() / 24)),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		HistoryArchivalUri:      types.StringValue(ns.Config.GetHistoryArchivalUri()),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		VisibilityArchivalUri:   types.StringValue(ns.Config.GetVisibilityArchivalUri()),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
	}

	// Save data into Terraform state
	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
