package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
)

var (
	_ resource.Resource                = &NamespaceResource{}
	_ resource.ResourceWithImportState = &NamespaceResource{}
)

func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

// NamespaceResource the resource implementation.
type NamespaceResource struct {
	client operatorservice.OperatorServiceClient
}

// NamespaceResourceModel describes the resource data model.
type NamespaceResourceModel struct {
	ConfigurableAttribute types.String `tfsdk:"configurable_attribute"`
	Defaulted             types.String `tfsdk:"defaulted"`
	Id                    types.String `tfsdk:"id"`
}

func (r *NamespaceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (r *NamespaceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Temporal Namespace resource",

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
				Optional:            true,
				Computed:            true,
			},
			"owner_email": schema.StringAttribute{
				MarkdownDescription: "Namespace Owner Email",
				Optional:            true,
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

func (r *NamespaceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal Namespace Resource")

	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	tflog.Info(ctx, "Configured Temporal Namespace client", map[string]any{"success": true})
	connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface), got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	client := operatorservice.NewOperatorServiceClient(connection)
	r.client = client

	tflog.Info(ctx, "Configured Temporal Namespace client", map[string]any{"success": true})
}

func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NamespaceResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create example, got error: %s", err))
	//     return
	// }

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	data.Id = types.StringValue("example-id")

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NamespaceResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NamespaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NamespaceResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update example, got error: %s", err))
	//     return
	// }

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NamespaceResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete example, got error: %s", err))
	//     return
	// }
}

func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
