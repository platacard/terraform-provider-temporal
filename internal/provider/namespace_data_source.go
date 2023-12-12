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
	Name                    string   `tfsdk:"name"`
	Id                      string   `tfsdk:"id"`
	Description             string   `tfsdk:"description"`
	OwnerEmail              string   `tfsdk:"owner_email"`
	State                   string   `tfsdk:"state"`
	ActiveClusterName       string   `tfsdk:"active_cluster_name"`
	Clusters                []string `tfsdk:"clusters"`
	HistoryArchivalState    string   `tfsdk:"history_archival_state"`
	VisibilityArchivalState string   `tfsdk:"visibility_archival_state"`
	IsGlobalNamespace       bool     `tfsdk:"is_global_namespace"`
	FailoverVersion         int      `tfsdk:"failover_version"`
	FailoverHistory         []string `tfsdk:"failover_history"`
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
			"state": schema.StringAttribute{
				MarkdownDescription: "State of Namespace",
				Computed:            true,
			},
			"active_cluster_name": schema.StringAttribute{
				MarkdownDescription: "Active Cluster Name",
				Computed:            true,
			},
			"clusters": schema.ListAttribute{
				MarkdownDescription: "Temporal Clusters",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"history_archival_state": schema.StringAttribute{
				MarkdownDescription: "History Archival State",
				Computed:            true,
			},
			"visibility_archival_state": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival State",
				Computed:            true,
			},
			"is_global_namespace": schema.BoolAttribute{
				MarkdownDescription: "Namespace is Global",
				Computed:            true,
			},
			"failover_version": schema.NumberAttribute{
				MarkdownDescription: "Failover Version",
				Computed:            true,
			},
			"failover_history": schema.ListAttribute{
				MarkdownDescription: "Failover History",
				ElementType:         types.StringType,
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

	var data *NamespaceDataSourceModel
	data = &NamespaceDataSourceModel{
		Name:                    ns.NamespaceInfo.GetName(),
		Id:                      ns.NamespaceInfo.GetId(),
		Description:             ns.NamespaceInfo.GetDescription(),
		OwnerEmail:              ns.NamespaceInfo.GetOwnerEmail(),
		State:                   enums.NamespaceState_name[int32(ns.NamespaceInfo.GetState())],
		ActiveClusterName:       ns.GetReplicationConfig().GetActiveClusterName(),
		HistoryArchivalState:    enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())],
		VisibilityArchivalState: enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())],
		IsGlobalNamespace:       ns.GetIsGlobalNamespace(),
		FailoverVersion:         int(ns.GetFailoverVersion()),
	}

	for _, clusters := range ns.GetReplicationConfig().GetClusters() {
		data.Clusters = append(data.Clusters, clusters.ClusterName)
	}
	for _, failover := range ns.GetFailoverHistory() {
		data.Clusters = append(data.Clusters, failover.String())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
