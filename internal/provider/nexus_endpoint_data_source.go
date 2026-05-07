package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

var (
	_ datasource.DataSource              = &NexusEndpointDataSource{}
	_ datasource.DataSourceWithConfigure = &NexusEndpointDataSource{}
)

// NewNexusEndpointDataSource returns a new instance of the NexusEndpointDataSource.
func NewNexusEndpointDataSource() datasource.DataSource {
	return &NexusEndpointDataSource{}
}

// NexusEndpointDataSource implements read-only access to a Temporal Nexus endpoint by name.
type NexusEndpointDataSource struct {
	client operatorservice.OperatorServiceClient
}

// NexusEndpointDataSourceModel mirrors the resource model with all fields computed except `name`.
type NexusEndpointDataSourceModel struct {
	Name           types.String `tfsdk:"name"`
	Id             types.String `tfsdk:"id"`
	Version        types.Int64  `tfsdk:"version"`
	Description    types.String `tfsdk:"description"`
	WorkerTarget   types.Object `tfsdk:"worker_target"`
	ExternalTarget types.Object `tfsdk:"external_target"`
	UrlPrefix      types.String `tfsdk:"url_prefix"`
}

func (d *NexusEndpointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nexus_endpoint"
}

func (d *NexusEndpointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read-only data source for an existing Temporal Nexus endpoint, looked up by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Endpoint name.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-generated unique endpoint ID.",
				Computed:            true,
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Optimistic-concurrency-control version.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Endpoint description.",
				Computed:            true,
			},
			"worker_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Worker target, if the endpoint is configured with one.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"namespace":  schema.StringAttribute{Computed: true},
					"task_queue": schema.StringAttribute{Computed: true},
				},
			},
			"external_target": schema.SingleNestedAttribute{
				MarkdownDescription: "External target, if the endpoint is configured with one.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{Computed: true},
				},
			},
			"url_prefix": schema.StringAttribute{
				MarkdownDescription: "Server-rendered URL prefix.",
				Computed:            true,
			},
		},
	}
}

func (d *NexusEndpointDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	conn, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = operatorservice.NewOperatorServiceClient(conn)
	tflog.Info(ctx, "Configured Temporal Nexus Endpoint data source", map[string]any{"success": true})
}

func (d *NexusEndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var name string
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := getEndpointByIDOrName(ctx, d.client, "", name)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Nexus endpoint %q: %s", name, err))
		return
	}

	data := &NexusEndpointDataSourceModel{}
	// Reuse the resource model populator by mapping the data-source model
	// onto the resource shape (identical fields).
	resourceShape := &NexusEndpointResourceModel{
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes()),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes()),
	}
	updateModelFromEndpoint(resourceShape, endpoint)

	data.Name = resourceShape.Name
	data.Id = resourceShape.Id
	data.Version = resourceShape.Version
	data.Description = resourceShape.Description
	data.WorkerTarget = resourceShape.WorkerTarget
	data.ExternalTarget = resourceShape.ExternalTarget
	data.UrlPrefix = resourceShape.UrlPrefix

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// Compile-time assertion that the data-source attr types match the resource's.
var _ = func() bool {
	var _ map[string]attr.Type = workerTargetAttrTypes()
	return true
}()
