package provider

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

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

	data.Id = types.StringValue(ns.NamespaceInfo.GetId())
	data.ActiveClusterName = types.StringValue(ns.GetReplicationConfig().GetActiveClusterName())
	data.HistoryArchivalUri = types.StringValue(ns.GetConfig().GetHistoryArchivalUri())
	data.VisibilityArchivalUri = types.StringValue(ns.GetConfig().GetVisibilityArchivalUri())

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

	data := &NamespaceResourceModel{
		Id:                      types.StringValue(ns.NamespaceInfo.GetId()),
		Name:                    state.Name,
		Description:             types.StringValue(ns.NamespaceInfo.GetDescription()),
		OwnerEmail:              types.StringValue(ns.NamespaceInfo.GetOwnerEmail()),
		Retention:               types.Int64Value(int64(ns.Config.WorkflowExecutionRetentionTtl.AsDuration().Hours() / 24)),
		ActiveClusterName:       types.StringValue(ns.GetReplicationConfig().GetActiveClusterName()),
		HistoryArchivalState:    types.StringValue(ns.Config.GetHistoryArchivalState().String()),
		HistoryArchivalUri:      types.StringValue(ns.Config.GetHistoryArchivalUri()),
		VisibilityArchivalState: types.StringValue(ns.Config.GetVisibilityArchivalState().String()),
		VisibilityArchivalUri:   types.StringValue(ns.Config.GetVisibilityArchivalUri()),
		IsGlobalNamespace:       types.BoolValue(ns.GetIsGlobalNamespace()),
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
		ReplicationConfig: &replication.NamespaceReplicationConfig{
			ActiveClusterName: data.ActiveClusterName.ValueString(),
		},
		// promote local namespace to global namespace. Ignored if namespace is already global namespace.
		PromoteNamespace: data.IsGlobalNamespace.ValueBool(),
	}

	ns, err := client.UpdateNamespace(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.NamespaceAlreadyExists); !ok {
			resp.Diagnostics.AddError("Request error", "namespace registration failed: "+err.Error())
			return
		} else {
			resp.Diagnostics.AddError(data.Name.ValueString(), "namespace is already registered: "+err.Error())
			return
		}
	}

	data.Id = types.StringValue(ns.NamespaceInfo.GetId())
	data.ActiveClusterName = types.StringValue(ns.GetReplicationConfig().GetActiveClusterName())
	data.HistoryArchivalUri = types.StringValue(ns.GetConfig().GetHistoryArchivalUri())
	data.VisibilityArchivalUri = types.StringValue(ns.GetConfig().GetVisibilityArchivalUri())
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
