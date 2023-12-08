package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/enums/v1"
	v12 "go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

const (
	DefaultNamespaceRetention = 3 * 24 * time.Hour
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
	Name                    types.String `tfsdk:"name"`
	Id                      types.String `tfsdk:"id"`
	Description             types.String `tfsdk:"description"`
	OwnerEmail              types.String `tfsdk:"owner_email"`
	State                   types.String `tfsdk:"state"`
	ActiveClusterName       types.String `tfsdk:"active_cluster_name"`
	Clusters                types.List   `tfsdk:"clusters"`
	HistoryArchivalState    types.String `tfsdk:"history_archival_state"`
	VisibilityArchivalState types.String `tfsdk:"visibility_archival_state"`
	IsGlobalNamespace       types.Bool   `tfsdk:"is_global_namespace"`
	FailoverVersion         types.Int64  `tfsdk:"failover_version"`
	FailoverHistory         types.List   `tfsdk:"failover_history"`
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
			"failover_version": schema.Int64Attribute{
				MarkdownDescription: "Failover Version",
				Computed:            true,
			},
			"failover_history": schema.ListAttribute{
				MarkdownDescription: "Failover History",
				ElementType:         types.StringType,
				Computed:            true,
				Default:             listdefault.StaticValue(types.ListNull(types.StringType)),
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

	reteniton := DefaultNamespaceRetention
	request := &workflowservice.RegisterNamespaceRequest{
		Namespace:                        data.Name.ValueString(),
		Description:                      data.Description.ValueString(),
		OwnerEmail:                       data.OwnerEmail.ValueString(),
		WorkflowExecutionRetentionPeriod: &reteniton,
		// Data:                             data.,
		// WorkflowExecutionRetentionPeriod: &retention,
		// Clusters:                         data.Clusters,
		ActiveClusterName: data.ActiveClusterName.ValueString(),
		// HistoryArchivalState:             archState,
		// HistoryArchivalUri:               c.String(FlagHistoryArchivalURI),
		// VisibilityArchivalState:          archVisState,
		// VisibilityArchivalUri:            c.String(FlagVisibilityArchivalURI),
		IsGlobalNamespace: data.IsGlobalNamespace.ValueBool(),
	}

	_, err := r.client.RegisterNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		} else {
			resp.Diagnostics.AddError(data.Name.ValueString(), "namespace is already registered: "+err.Error())
			return
		}
	}

	tflog.Info(ctx, fmt.Sprintf("The namespace: %s is successfully registered", data.Name))
	tflog.Trace(ctx, "created a resource")

	ns, err := r.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace info, got error: %s", err))
		return
	}

	data = NamespaceResourceModel{
		Name:                    types.StringValue(ns.NamespaceInfo.GetName()),
		Id:                      types.StringValue(ns.NamespaceInfo.GetId()),
		Description:             types.StringValue(ns.NamespaceInfo.GetDescription()),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.GetOwnerEmail()),
		State:                   types.StringValue(enums.NamespaceState_name[int32(ns.NamespaceInfo.GetState())]),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
		FailoverVersion:         types.Int64Value(ns.GetFailoverVersion()),
	}
	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

	}
	failoverHistory := ns.GetFailoverHistory()
	if failoverHistory != nil {
		for _, failover := range ns.GetFailoverHistory() {
			var history []string
			history = append(history, failover.String())
			data.FailoverHistory, _ = types.ListValueFrom(ctx, types.StringType, history)
		}
	} else {
		data.FailoverHistory = types.ListNull(types.StringType)
	}

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
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace info, got error: %s", err))
		return
	}

	data = NamespaceResourceModel{
		Name:                    types.StringValue(ns.NamespaceInfo.GetName()),
		Id:                      types.StringValue(ns.NamespaceInfo.GetId()),
		Description:             types.StringValue(ns.NamespaceInfo.GetDescription()),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.GetOwnerEmail()),
		State:                   types.StringValue(enums.NamespaceState_name[int32(ns.NamespaceInfo.GetState())]),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
		FailoverVersion:         types.Int64Value(ns.GetFailoverVersion()),
	}

	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

	}
	failoverHistory := ns.GetFailoverHistory()
	if failoverHistory != nil {
		for _, failover := range ns.GetFailoverHistory() {
			var history []string
			history = append(history, failover.String())
			data.FailoverHistory, _ = types.ListValueFrom(ctx, types.StringType, history)
		}
	} else {
		data.FailoverHistory = types.ListNull(types.StringType)
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

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// reteniton := DefaultNamespaceRetention
	request := &workflowservice.UpdateNamespaceRequest{
		Namespace: data.Name.ValueString(),
		UpdateInfo: &v12.UpdateNamespaceInfo{
			Description: data.Description.ValueString(),
		},
		Config:            &v12.NamespaceConfig{},
		ReplicationConfig: &replication.NamespaceReplicationConfig{},
		// promote local namespace to global namespace. Ignored if namespace is already global namespace.
		PromoteNamespace: false,
	}
	// Namespace:                        data.Name.ValueString(),
	// Description:                      data.Description.ValueString(),
	// OwnerEmail:                       data.OwnerEmail.ValueString(),
	// WorkflowExecutionRetentionPeriod: &reteniton,
	// // Data:                             data.,
	// // WorkflowExecutionRetentionPeriod: &retention,
	// // Clusters:                         data.Clusters,
	// ActiveClusterName: data.ActiveClusterName.ValueString(),
	// // HistoryArchivalState:             archState,
	// // HistoryArchivalUri:               c.String(FlagHistoryArchivalURI),
	// // VisibilityArchivalState:          archVisState,
	// // VisibilityArchivalUri:            c.String(FlagVisibilityArchivalURI),
	// IsGlobalNamespace: data.IsGlobalNamespace.ValueBool(),
	// }

	_, err := r.client.UpdateNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		} else {
			resp.Diagnostics.AddError(data.Name.ValueString(), "namespace is already registered: "+err.Error())
			return
		}
	}

	tflog.Info(ctx, fmt.Sprintf("The namespace: %s is successfully registered", data.Name))
	tflog.Trace(ctx, "created a resource")

	ns, err := r.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace info, got error: %s", err))
		return
	}

	data = NamespaceResourceModel{
		Name:                    types.StringValue(ns.NamespaceInfo.GetName()),
		Id:                      types.StringValue(ns.NamespaceInfo.GetId()),
		Description:             types.StringValue(ns.NamespaceInfo.GetDescription()),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.GetOwnerEmail()),
		State:                   types.StringValue(enums.NamespaceState_name[int32(ns.NamespaceInfo.GetState())]),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
		FailoverVersion:         types.Int64Value(ns.GetFailoverVersion()),
	}
	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

	}
	failoverHistory := ns.GetFailoverHistory()
	if failoverHistory != nil {
		for _, failover := range ns.GetFailoverHistory() {
			var history []string
			history = append(history, failover.String())
			data.FailoverHistory, _ = types.ListValueFrom(ctx, types.StringType, history)
		}
	} else {
		data.FailoverHistory = types.ListNull(types.StringType)
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
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
