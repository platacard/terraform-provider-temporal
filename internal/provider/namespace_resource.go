package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/enums/v1"
	v12 "go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

const (
	day = 24 * time.Hour
)

var (
	_ resource.Resource                = &NamespaceResource{}
	_ resource.ResourceWithConfigure   = &NamespaceResource{}
	_ resource.ResourceWithImportState = &NamespaceResource{}
)

func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

// NamespaceResource the resource implementation.
type NamespaceResource struct {
	client grpc.ClientConnInterface
}

// NamespaceResourceModel describes the resource data model.
type NamespaceResourceModel struct {
	Name        types.String `tfsdk:"name"`
	Id          types.String `tfsdk:"id"`
	Description types.String `tfsdk:"description"`
	OwnerEmail  types.String `tfsdk:"owner_email"`
	Retention   types.Int64  `tfsdk:"retention"`
	// State                   types.String `tfsdk:"state"`
	ActiveClusterName       types.String `tfsdk:"active_cluster_name"`
	Clusters                types.List   `tfsdk:"clusters"`
	HistoryArchivalState    types.String `tfsdk:"history_archival_state"`
	HistoryArchivalUri      types.String `tfsdk:"history_archival_uri"`
	VisibilityArchivalState types.String `tfsdk:"visibility_archival_state"`
	VisibilityArchivalUri   types.String `tfsdk:"visibility_archival_uri"`
	IsGlobalNamespace       types.Bool   `tfsdk:"is_global_namespace"`
	// FailoverVersion         types.Int64  `tfsdk:"failover_version"`
	// FailoverHistory         types.List   `tfsdk:"failover_history"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Namespace Description",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"owner_email": schema.StringAttribute{
				MarkdownDescription: "Namespace Owner Email",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("hurma@dif.tech"),
			},
			"retention": schema.Int64Attribute{
				MarkdownDescription: "Workflow Execution retention",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3),
			},
			"active_cluster_name": schema.StringAttribute{
				MarkdownDescription: "Active Cluster Name",
				Optional:            true,
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
				Optional:            true,
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
			},
			"is_global_namespace": schema.BoolAttribute{
				MarkdownDescription: "Namespace is Global",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
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
	client, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface), got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client

	tflog.Info(ctx, "Configured Temporal Namespace client", map[string]any{"success": true})
}

func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NamespaceResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	retention := time.Duration(data.Retention.ValueInt64()) * day

	request := &workflowservice.RegisterNamespaceRequest{
		Namespace:                        data.Name.ValueString(),
		Description:                      data.Description.ValueString(),
		OwnerEmail:                       data.OwnerEmail.ValueString(),
		WorkflowExecutionRetentionPeriod: &retention,
		// Data:                             data.,
		// Clusters:                         data.Clusters,
		ActiveClusterName: data.ActiveClusterName.ValueString(),
		// HistoryArchivalState:             archState,
		// HistoryArchivalUri:               c.String(FlagHistoryArchivalURI),
		// VisibilityArchivalState:          archVisState,
		// VisibilityArchivalUri:            c.String(FlagVisibilityArchivalURI),
		IsGlobalNamespace: data.IsGlobalNamespace.ValueBool(),
	}
	_, err := client.RegisterNamespace(ctx, request)
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

	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
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
		Retention:               types.Int64Value(int64(ns.Config.WorkflowExecutionRetentionTtl.Hours() / 24)),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		HistoryArchivalUri:      types.StringValue(ns.Config.GetHistoryArchivalUri()),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		VisibilityArchivalUri:   types.StringValue(ns.Config.GetVisibilityArchivalUri()),
		IsGlobalNamespace:       data.IsGlobalNamespace,
	}
	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NamespaceResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
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
		Retention:               types.Int64Value(int64(ns.Config.WorkflowExecutionRetentionTtl.Hours() / 24)),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		HistoryArchivalUri:      types.StringValue(ns.Config.GetHistoryArchivalUri()),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		VisibilityArchivalUri:   types.StringValue(ns.Config.GetVisibilityArchivalUri()),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
	}

	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

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

	client := workflowservice.NewWorkflowServiceClient(r.client)

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	retention := time.Duration(data.Retention.ValueInt64()) * day

	// reteniton := DefaultNamespaceRetention
	request := &workflowservice.UpdateNamespaceRequest{
		Namespace: data.Name.ValueString(),
		UpdateInfo: &v12.UpdateNamespaceInfo{
			// Description string `protobuf:"bytes,1,opt,name=description,proto3" json:"description,omitempty"`
			// OwnerEmail  string `protobuf:"bytes,2,opt,name=owner_email,json=ownerEmail,proto3" json:"owner_email,omitempty"`
			// // A key-value map for any customized purpose.
			// // If data already exists on the namespace,
			// // this will merge with the existing key values.
			// Data map[string]string `protobuf:"bytes,3,rep,name=data,proto3" json:"data,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
			// // New namespace state, server will reject if transition is not allowed.
			// // Allowed transitions are:
			// //  Registered -> [ Deleted | Deprecated | Handover ]
			// //  Handover -> [ Registered ]
			// // Default is NAMESPACE_STATE_UNSPECIFIED which is do not change state.
			// State v1.NamespaceState `protobuf:"varint,4,opt,name=state,proto3,enum=temporal.api.enums.v1.NamespaceState" json:"state,omitempty"`

			Description: data.Description.ValueString(),
			OwnerEmail:  data.OwnerEmail.ValueString(),
		},
		Config: &v12.NamespaceConfig{
			WorkflowExecutionRetentionTtl: &retention,
			VisibilityArchivalState:       enums.ArchivalState(enums.ArchivalState_value[data.VisibilityArchivalState.ValueString()]),
			VisibilityArchivalUri:         data.VisibilityArchivalUri.ValueString(),
			HistoryArchivalState:          enums.ArchivalState(enums.ArchivalState_value[data.HistoryArchivalState.ValueString()]),
			HistoryArchivalUri:            data.HistoryArchivalUri.ValueString(),
			// CustomSearchAttributeAliases map[string]string `protobuf:"bytes,7,rep,name=custom_search_attribute_aliases,json=customSearchAttributeAliases,proto3" json:"custom_search_attribute_aliases,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
		},
		ReplicationConfig: &replication.NamespaceReplicationConfig{
			// ActiveClusterName string                      `protobuf:"bytes,1,opt,name=active_cluster_name,json=activeClusterName,proto3" json:"active_cluster_name,omitempty"`
			// Clusters          []*ClusterReplicationConfig `protobuf:"bytes,2,rep,name=clusters,proto3" json:"clusters,omitempty"`
			// State             v1.ReplicationState         `protobuf:"varint,3,opt,name=state,proto3,enum=temporal.api.enums.v1.ReplicationState" json:"state,omitempty"`
		},
		// promote local namespace to global namespace. Ignored if namespace is already global namespace.
		PromoteNamespace: data.IsGlobalNamespace.ValueBool(),
	}

	_, err := client.UpdateNamespace(ctx, request)
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

	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
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
		Retention:               types.Int64Value(int64(ns.Config.WorkflowExecutionRetentionTtl.Hours() / 24)),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())]),
		HistoryArchivalUri:      types.StringValue(ns.Config.GetHistoryArchivalUri()),
		VisibilityArchivalState: types.StringValue(enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())]),
		VisibilityArchivalUri:   types.StringValue(ns.Config.GetVisibilityArchivalUri()),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
	}
	for _, cluster := range ns.GetReplicationConfig().GetClusters() {
		var clusters []string
		clusters = append(clusters, cluster.ClusterName)
		data.Clusters, _ = types.ListValueFrom(ctx, types.StringType, clusters)

	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NamespaceResourceModel

	client := operatorservice.NewOperatorServiceClient(r.client)

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := client.DeleteNamespace(ctx, &operatorservice.DeleteNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		switch err.(type) {
		case *serviceerror.NamespaceNotFound:
			resp.Diagnostics.AddError("Request error", "Namespace not found: "+err.Error())
			return
		default:
			resp.Diagnostics.AddError("Request error", "Unable to delete namespace: "+err.Error())
		}
	}
}

func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}
