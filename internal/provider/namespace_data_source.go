package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ datasource.DataSource              = &NamespaceDataSource{}
	_ datasource.DataSourceWithConfigure = &NamespaceDataSource{}
)

func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// NamespaceDataSource defines the data source implementation.
type NamespaceDataSource struct {
	client workflowservice.WorkflowServiceClient
}

// NamespaceDataSourceModel describes the data source data model.
type NamespaceDataSourceModel struct {
	Name                    types.String   `tfsdk:"name"`
	Id                      types.String   `tfsdk:"id"`
	Description             types.String   `tfsdk:"description"`
	OwnerEmail              types.String   `tfsdk:"owner_email"`
	State                   types.String   `tfsdk:"state"`
	ActiveClusterName       types.String   `tfsdk:"active_cluster_name"`
	Clusters                []types.String `tfsdk:"clusters"`
	HistoryArchivalState    types.String   `tfsdk:"history_archival_state"`
	VisibilityArchivalState types.String   `tfsdk:"visibility_archival_state"`
	IsGlobalNamespace       types.Bool     `tfsdk:"is_global_namespace"`
	FailoverVersion         types.Int64    `tfsdk:"failover_version"`
	FailoverHistory         []types.String `tfsdk:"failover_history"`
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
			"Description": schema.StringAttribute{
				MarkdownDescription: "Namespace Description",
				Optional:            true,
			},
			"OwnerEmail": schema.StringAttribute{
				MarkdownDescription: "Namespace Owner Email",
				Optional:            true,
			},
			"State": schema.StringAttribute{
				MarkdownDescription: "State of Namespace",
				Optional:            true,
			},
			"ActiveClusterName": schema.StringAttribute{
				MarkdownDescription: "Active Cluster Name",
				Optional:            true,
			},
			"Clusters": schema.ListAttribute{
				MarkdownDescription: "Temporal Clusters",
				Optional:            true,
			},
			"HistoryArchivalState": schema.StringAttribute{
				MarkdownDescription: "History Archival State",
				Optional:            true,
			},
			"VisibilityArchivalState": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival State",
				Optional:            true,
			},
			"IsGlobalNamespace": schema.BoolAttribute{
				MarkdownDescription: "Namespace is Global",
				Optional:            true,
			},
			"FailoverVersion": schema.Int64Attribute{
				MarkdownDescription: "Failover Version",
				Optional:            true,
			},
			"FailoverHistory": schema.ListAttribute{
				MarkdownDescription: "Failover History",
				Optional:            true,
			},
		},
	}
}

func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
}

func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NamespaceDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	ns, err := d.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
		Id:        data.Id.String(),
	})
	data = NamespaceDataSourceModel{
		Name:                    types.StringValue(ns.NamespaceInfo.Name),
		Id:                      types.StringValue(ns.NamespaceInfo.Id),
		Description:             types.StringValue(ns.NamespaceInfo.Description),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.OwnerEmail),
		State:                   types.StringValue(ns.NamespaceInfo.State.String()),
		ActiveClusterName:       types.StringValue(ns.ReplicationConfig.ActiveClusterName),
		HistoryArchivalState:    types.StringValue(ns.Config.HistoryArchivalState.String()),
		VisibilityArchivalState: types.StringValue(ns.Config.VisibilityArchivalState.String()),
		IsGlobalNamespace:       types.BoolValue(ns.IsGlobalNamespace),
		FailoverVersion:         types.Int64Value(ns.FailoverVersion),
	}

	for _, clusters := range ns.ReplicationConfig.Clusters {
		data.Clusters = append(data.Clusters, types.StringValue(clusters.ClusterName))
	}
	for _, failover := range ns.FailoverHistory {
		data.Clusters = append(data.Clusters, types.StringValue(failover.String()))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace, got error: %s", err))
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
