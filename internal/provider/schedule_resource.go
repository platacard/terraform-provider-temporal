package provider

import (
	"context"

	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"google.golang.org/grpc"
)

var (
	_ resource.Resource                = &ScheduleResource{}
	_ resource.ResourceWithConfigure   = &ScheduleResource{}
	_ resource.ResourceWithImportState = &ScheduleResource{}
)

// NewScheduleResource creates a new instance of ScheduleResource.
func NewScheduleResource() resource.Resource {
	return &ScheduleResource{}
}

// ScheduleResource implements the Temporal schedule resource.
type ScheduleResource struct {
	client grpc.ClientConnInterface
}

// Metadata sets the metadata for the schedule resource.
func (r *ScheduleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

// Schema returns the schema for the Temporal schedule resource.
func (r *ScheduleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// TODO: add Schema
	}
}

// Configure sets up the schedule resource configuration.
func (r *ScheduleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal Schedule Resource")

	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
	tflog.Info(ctx, "Configured Temporal Schedule client", map[string]any{"success": true})
}

// Create creates a new schedule in Temporal.
func (r *ScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// TODO: Implerment Create
}

// Read reads the current state of a schedule.
func (r *ScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// TODO: Implement Read
}

// Update updates an existing schedule.
func (r *ScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// TODO: Implement Update
}

// Delete deletes a schedule.
func (r *ScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// TODO: Implement Delete
}

// ImportState imports an existing schedule into Terraform state.
func (r *ScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// TODO: Implement ImportState
}
