package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
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
	client workflowservice.WorkflowServiceClient
}

// NamespaceResourceModel describes the resource data model.
type NamespaceResourceModel struct {
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

	client := workflowservice.NewWorkflowServiceClient(connection)
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

	request := &workflowservice.RegisterNamespaceRequest{
		Namespace:   data.Name,
		Description: data.Description,
		OwnerEmail:  data.OwnerEmail,
		// Data:                             data.,
		// WorkflowExecutionRetentionPeriod: &retention,
		// Clusters:                         data.Clusters,
		ActiveClusterName: data.ActiveClusterName,
		// HistoryArchivalState:             archState,
		// HistoryArchivalUri:               c.String(FlagHistoryArchivalURI),
		// VisibilityArchivalState:          archVisState,
		// VisibilityArchivalUri:            c.String(FlagVisibilityArchivalURI),
		IsGlobalNamespace: data.IsGlobalNamespace,
	}

	_, err := r.client.RegisterNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		} else {
			resp.Diagnostics.AddError(data.Name, "namespace is already registered: "+err.Error())
			return
		}
	}

	tflog.Info(ctx, fmt.Sprintf("The namespace: %s is successfully registered", data.Name))
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NamespaceResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ns, err := r.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name,
		Id:        data.Id,
	})

	data = NamespaceResourceModel{
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

	tflog.Trace(ctx, "read a Temporal Namespace resource")

	// Set refreshed state
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NamespaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NamespaceResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// var data NamespaceResourceModel

	// connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	// if !ok {
	// 	resp.Diagnostics.AddError(
	// 		"Unexpected Data Source Configure Type",
	// 		fmt.Sprintf("Expected grpc.ClientConnInterface), got: %T. Please report this issue to the provider developers.", req.ProviderData),
	// 	)

	// 	return
	// }

	// client := operatorservice.NewOperatorServiceClient(connection)

	// _, err := client.DeleteNamespace(ctx, &operatorservice.DeleteNamespaceRequest{
	// 	Namespace: ns,
	// })
	// if err != nil {
	// 	switch err.(type) {
	// 	case *serviceerror.NamespaceNotFound:
	// 		resp.Diagnostics.AddError("Request error", "Namespace not found: "+err.Error())
	// 		return
	// 	default:
	// 		resp.Diagnostics.AddError("Request error", "Unable to delete namespace: "+err.Error())
	// 	}
	// }
	// // Read Terraform prior state data into the model
	// resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	// if resp.Diagnostics.HasError() {
	// 	return
	// }
}

func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
