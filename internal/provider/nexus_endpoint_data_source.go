package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

var (
	_ datasource.DataSource              = &NexusEndpointDataSource{}
	_ datasource.DataSourceWithConfigure = &NexusEndpointDataSource{}
)

func NewNexusEndpointDataSource() datasource.DataSource {
	return &NexusEndpointDataSource{}
}

type NexusEndpointDataSource struct {
	client operatorservice.OperatorServiceClient
}

func (d *NexusEndpointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nexus_endpoint"
}

func (d *NexusEndpointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Temporal Nexus Endpoint data source",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Endpoint name, unique within the cluster",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-generated endpoint identifier",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Endpoint description in Markdown",
				Computed:            true,
			},
			"worker_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Set when requests are routed to a task queue polled by Nexus workers",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"namespace": schema.StringAttribute{
						MarkdownDescription: "Namespace to route requests to",
						Computed:            true,
					},
					"task_queue": schema.StringAttribute{
						MarkdownDescription: "Nexus task queue to route requests to",
						Computed:            true,
					},
				},
			},
			"external_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Set when requests are routed to an external Nexus server",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						MarkdownDescription: "URL to call",
						Computed:            true,
					},
				},
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Data version of the endpoint, incremented by the server on every update",
				Computed:            true,
			},
			"url_prefix": schema.StringAttribute{
				MarkdownDescription: "Server URL path prefix for invoking operations on this endpoint. It does not change when the endpoint is renamed",
				Computed:            true,
			},
		},
	}
}

func (d *NexusEndpointDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.client = operatorservice.NewOperatorServiceClient(connection)
}

func (d *NexusEndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NexusEndpointModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := findNexusEndpointByName(ctx, d.client, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Nexus Endpoint",
			fmt.Sprintf("Could not read Nexus endpoint %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(data.setFromEndpoint(endpoint)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
