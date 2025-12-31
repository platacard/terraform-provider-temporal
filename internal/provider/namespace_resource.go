package provider

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

const (
	// day represents the number of nanoseconds in a day, used for time calculations.
	day = 24 * time.Hour
)

var (
	_ resource.Resource                = &NamespaceResource{}
	_ resource.ResourceWithConfigure   = &NamespaceResource{}
	_ resource.ResourceWithImportState = &NamespaceResource{}
)

// NewNamespaceResource creates a new instance of NamespaceResource.
func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

// NamespaceResource a Temporal namespace resource implementation.
type NamespaceResource struct {
	client grpc.ClientConnInterface
}

// NamespaceResourceModel defines the data schema for a Temporal namespace resource.
type NamespaceResourceModel struct {
	Name                    types.String `tfsdk:"name"`
	Id                      types.String `tfsdk:"id"`
	Description             types.String `tfsdk:"description"`
	OwnerEmail              types.String `tfsdk:"owner_email"`
	Retention               types.Int64  `tfsdk:"retention"`
	ActiveClusterName       types.String `tfsdk:"active_cluster_name"`
	Clusters                types.List   `tfsdk:"clusters"`
	HistoryArchivalState    types.String `tfsdk:"history_archival_state"`
	HistoryArchivalUri      types.String `tfsdk:"history_archival_uri"`
	VisibilityArchivalState types.String `tfsdk:"visibility_archival_state"`
	VisibilityArchivalUri   types.String `tfsdk:"visibility_archival_uri"`
	IsGlobalNamespace       types.Bool   `tfsdk:"is_global_namespace"`
}

// Metadata sets the metadata for the namespace resource, specifically the type name.
func (r *NamespaceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

// Schema returns the schema for the Temporal namespace resource.
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
				Required:            true,
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
				MarkdownDescription: "Clusters",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"history_archival_state": schema.StringAttribute{
				MarkdownDescription: "History Archival State",
				Computed:            true,
				Optional:            true,
				Default:             stringdefault.StaticString(enums.ARCHIVAL_STATE_DISABLED.String()),
			},
			"history_archival_uri": schema.StringAttribute{
				MarkdownDescription: "History Archival URI",
				Computed:            true,
				Optional:            true,
			},
			"visibility_archival_state": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival State",
				Computed:            true,
				Optional:            true,
				Default:             stringdefault.StaticString(enums.ARCHIVAL_STATE_DISABLED.String()),
			},
			"visibility_archival_uri": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival URI",
				Computed:            true,
				Optional:            true,
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

// Configure sets up the namespace resource configuration.
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

// Create is responsible for creating a new namespace in Temporal.
func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NamespaceResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	retention := durationpb.New(time.Duration(data.Retention.ValueInt64()) * day)

	request := &workflowservice.RegisterNamespaceRequest{
		Namespace:                        data.Name.ValueString(),
		Description:                      data.Description.ValueString(),
		OwnerEmail:                       data.OwnerEmail.ValueString(),
		WorkflowExecutionRetentionPeriod: retention,
		ActiveClusterName:                data.ActiveClusterName.ValueString(),
		VisibilityArchivalState:          ArchivalState[data.VisibilityArchivalState.ValueString()],
		VisibilityArchivalUri:            data.VisibilityArchivalUri.ValueString(),
		HistoryArchivalState:             ArchivalState[data.HistoryArchivalState.ValueString()],
		HistoryArchivalUri:               data.HistoryArchivalUri.ValueString(),
		IsGlobalNamespace:                data.IsGlobalNamespace.ValueBool(),
	}

	clusters, diags := getClustersFromModel(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(clusters) > 0 {
		request.Clusters = clusters
	}

	_, err := client.RegisterNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		}
		resp.Diagnostics.AddError(data.Name.ValueString(), "namespace is already registered: "+err.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("The namespace: %s is successfully registered", data.Name))
	tflog.Trace(ctx, "created a resource")

	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create Namespace info, got error: %s", err))
		return
	}

	resp.Diagnostics.Append(updateModelFromSpec(ctx, &data, ns)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read is responsible for reading the current state of a Temporal namespace.
// It fetches the current configuration of the namespace and updates the Terraform state.
func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NamespaceResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	namespace := state.Name.ValueString()
	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: namespace,
	})
	if err != nil {
		errCode := status.Code(err)
		if errCode == codes.NotFound {
			tflog.Warn(ctx, "Namespace not found", map[string]interface{}{"err": err, "namespace": namespace})
			return
		} else {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace info, got error: %s", err))
			return
		}
	}

	tflog.Trace(ctx, "read a Temporal Namespace resource")

	var data NamespaceResourceModel
	resp.Diagnostics.Append(updateModelFromSpec(ctx, &data, ns)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update modifies an existing Temporal namespace based on Terraform configuration changes.
func (r *NamespaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NamespaceResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	retention := durationpb.New(time.Duration(data.Retention.ValueInt64()) * day)

	request := &workflowservice.UpdateNamespaceRequest{
		Namespace: data.Name.ValueString(),
		UpdateInfo: &namespace.UpdateNamespaceInfo{
			Description: data.Description.ValueString(),
			OwnerEmail:  data.OwnerEmail.ValueString(),
		},
		Config: &namespace.NamespaceConfig{
			WorkflowExecutionRetentionTtl: retention,
			VisibilityArchivalState:       ArchivalState[data.VisibilityArchivalState.ValueString()],
			VisibilityArchivalUri:         data.VisibilityArchivalUri.ValueString(),
			HistoryArchivalState:          ArchivalState[data.HistoryArchivalState.ValueString()],
			HistoryArchivalUri:            data.HistoryArchivalUri.ValueString(),
		},
		// promote local namespace to global namespace. Ignored if namespace is already global namespace.
		PromoteNamespace: data.IsGlobalNamespace.ValueBool(),
	}
	// get the current namespace info
	currentNs, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to describe namespace, got error: %s", err))
		return
	}
	// get the old namespace info to compare with the new namespace info
	var oldData NamespaceResourceModel
	resp.Diagnostics.Append(updateModelFromSpec(ctx, &oldData, currentNs)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// check if the clusters are changed
	if !data.Clusters.IsUnknown() && !data.Clusters.Equal(oldData.Clusters) {
		// check if the active cluster name is changed
		if !data.ActiveClusterName.IsUnknown() && !data.ActiveClusterName.Equal(oldData.ActiveClusterName) {
			// cannot update active cluster name and clusters at the same time
			resp.Diagnostics.AddError("Cannot update namespace", "Cannot update active cluster name and clusters at the same time")
			return
		}
		// check if the active cluster name is in the clusters
		if !clusterInClusters(oldData.ActiveClusterName, data.Clusters) {
			// cannot update active cluster name if it is not in the clusters
			resp.Diagnostics.AddError("Cannot update namespace", "Active cluster name is not in the clusters")
			return
		}
		// get the clusters from the model
		clusters, diags := getClustersFromModel(ctx, data)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(clusters) > 0 {
			request.ReplicationConfig = &replication.NamespaceReplicationConfig{
				Clusters: clusters,
			}
		}
	} else {
		// check if the active cluster name is in the clusters
		if !data.ActiveClusterName.IsUnknown() && !clusterInClusters(data.ActiveClusterName, oldData.Clusters) {
			// cannot update active cluster name if it is not in the clusters
			resp.Diagnostics.AddError("Cannot update namespace", "Active cluster name is not in the clusters")
			return
		}
	}

	_, err = client.UpdateNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		} else {
			resp.Diagnostics.AddError(data.Name.ValueString(), "namespace is already registered: "+err.Error())
			return
		}
	}

	// check if the active cluster name is changed
	if !data.ActiveClusterName.IsUnknown() && !data.ActiveClusterName.Equal(oldData.ActiveClusterName) {
		newRequest := &workflowservice.UpdateNamespaceRequest{
			Namespace: data.Name.ValueString(),
			ReplicationConfig: &replication.NamespaceReplicationConfig{
				ActiveClusterName: data.ActiveClusterName.ValueString(),
			},
		}
		_, err = client.UpdateNamespace(ctx, newRequest)
		if err != nil {
			if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
				resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
				return
			}
			resp.Diagnostics.AddError("Request error", "Unable to update namespace: "+err.Error())
			return
		}
	}
	// get the updated namespace info
	ns, err := client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: data.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to describe namespace, got error: %s", err))
		return
	}
	// update the model from the updated namespace info
	resp.Diagnostics.Append(updateModelFromSpec(ctx, &data, ns)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("The namespace: %s is successfully registered", data.Name))
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete removes a Temporal namespace from both Temporal and the Terraform state.
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

// ImportState allows existing Temporal namespaces to be imported into the Terraform state.
func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// getClustersFromModel gets the clusters from the model and the clusters in the request format.
func getClustersFromModel(ctx context.Context, model NamespaceResourceModel) ([]*replication.ClusterReplicationConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	clusters := make([]types.String, 0, len(model.Clusters.Elements()))
	diags.Append(model.Clusters.ElementsAs(ctx, &clusters, true)...)
	if diags.HasError() {
		return nil, diags
	}
	requestClusters := make([]*replication.ClusterReplicationConfig, 0, len(clusters))
	for _, cluster := range clusters {
		requestClusters = append(requestClusters, &replication.ClusterReplicationConfig{
			ClusterName: cluster.ValueString(),
		})
	}
	return requestClusters, diags
}

// getClustersFromRequest gets the clusters from the request and returns the clusters in the model format.
func getClustersFromRequest(ctx context.Context, clusterReplicationConfig []*replication.ClusterReplicationConfig) (types.List, diag.Diagnostics) {
	clusters := make([]types.String, 0, len(clusterReplicationConfig))
	for _, cluster := range clusterReplicationConfig {
		clusters = append(clusters, types.StringValue(cluster.GetClusterName()))
	}
	return types.ListValueFrom(ctx, types.StringType, clusters)
}

// updateModelFromSpec updates the model from the namespace spec.
func updateModelFromSpec(ctx context.Context, data *NamespaceResourceModel, ns *workflowservice.DescribeNamespaceResponse) diag.Diagnostics {
	var diags diag.Diagnostics
	data.Id = types.StringValue(ns.NamespaceInfo.GetId())
	data.Name = types.StringValue(ns.NamespaceInfo.GetName())
	data.Description = types.StringValue(ns.NamespaceInfo.GetDescription())
	data.OwnerEmail = types.StringValue(ns.NamespaceInfo.GetOwnerEmail())
	data.Retention = types.Int64Value(int64(ns.Config.GetWorkflowExecutionRetentionTtl().AsDuration().Hours() / 24))
	data.ActiveClusterName = types.StringValue(ns.GetReplicationConfig().GetActiveClusterName())
	data.HistoryArchivalState = types.StringValue(ns.GetConfig().GetHistoryArchivalState().String())
	data.HistoryArchivalUri = types.StringValue(ns.GetConfig().GetHistoryArchivalUri())
	data.VisibilityArchivalState = types.StringValue(ns.GetConfig().GetVisibilityArchivalState().String())
	data.VisibilityArchivalUri = types.StringValue(ns.GetConfig().GetVisibilityArchivalUri())
	data.IsGlobalNamespace = types.BoolValue(ns.GetIsGlobalNamespace())

	if len(ns.GetReplicationConfig().GetClusters()) > 0 {
		clustersList, clustersDiags := getClustersFromRequest(ctx, ns.GetReplicationConfig().GetClusters())
		diags.Append(clustersDiags...)
		if diags.HasError() {
			return diags
		}
		data.Clusters = clustersList
	}
	return diags
}

// clusterInClusters checks if the cluster is in the clusters list.
func clusterInClusters(clusterName types.String, clusters types.List) bool {
	for _, cluster := range clusters.Elements() {
		if clusterName.Equal(cluster) {
			return true
		}
	}
	return false
}
