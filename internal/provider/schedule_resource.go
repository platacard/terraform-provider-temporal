package provider

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"fmt"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	commonv1 "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	workflowv1 "go.temporal.io/api/workflow/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// defaultCatchupWindow default time window for catching up on missed schedules.
	defaultCatchupWindow = "5m"
)

var (
	_ resource.Resource                = &ScheduleResource{}
	_ resource.ResourceWithConfigure   = &ScheduleResource{}
	_ resource.ResourceWithImportState = &ScheduleResource{}
)

// ScheduleOverlapPolicy defines the valid overlap policy values.
var ScheduleOverlapPolicy = map[string]enums.ScheduleOverlapPolicy{
	"Skip":           enums.SCHEDULE_OVERLAP_POLICY_SKIP,
	"BufferOne":      enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
	"BufferAll":      enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ALL,
	"CancelOther":    enums.SCHEDULE_OVERLAP_POLICY_CANCEL_OTHER,
	"TerminateOther": enums.SCHEDULE_OVERLAP_POLICY_TERMINATE_OTHER,
	"AllowAll":       enums.SCHEDULE_OVERLAP_POLICY_ALLOW_ALL,
}

// ScheduleResourceModel defines the data schema for a Temporal schedule resource.
type ScheduleResourceModel struct {
	Namespace  types.String         `tfsdk:"namespace"`
	ScheduleID types.String         `tfsdk:"schedule_id"`
	Memo       types.Map            `tfsdk:"memo"`
	Spec       *ScheduleSpecModel   `tfsdk:"spec"`
	Action     *ScheduleActionModel `tfsdk:"action"`
	State      *ScheduleStateModel  `tfsdk:"state"`
	Policy     *SchedulePolicyModel `tfsdk:"policy_config"`
}

// ScheduleSpecModel defines the schedule specification.
type ScheduleSpecModel struct {
	Intervals     []IntervalModel `tfsdk:"intervals"`
	CalendarItems []CalendarModel `tfsdk:"calendar_items"`
	CronItems     []types.String  `tfsdk:"cron_items"`
	StartTime     types.String    `tfsdk:"start_time"`
	EndTime       types.String    `tfsdk:"end_time"`
	Jitter        types.String    `tfsdk:"jitter"`
	TimeZone      types.String    `tfsdk:"time_zone"`
}

// CalendarModel defines a calendar expression.
type CalendarModel struct {
	Year       types.String `tfsdk:"year"`
	Month      types.String `tfsdk:"month"`
	DayOfMonth types.String `tfsdk:"day_of_month"`
	DayOfWeek  types.String `tfsdk:"day_of_week"`
	Hour       types.String `tfsdk:"hour"`
	Minute     types.String `tfsdk:"minute"`
	Second     types.String `tfsdk:"second"`
	Comment    types.String `tfsdk:"comment"`
}

// IntervalModel defines an interval specification.
type IntervalModel struct {
	Every  types.String `tfsdk:"every"`
	Offset types.String `tfsdk:"offset"`
}

// ScheduleActionModel defines the action to be taken when the schedule triggers.
type ScheduleActionModel struct {
	Workflow *WorkflowActionModel `tfsdk:"workflow"`
}

// WorkflowActionModel defines the workflow action.
type WorkflowActionModel struct {
	WorkflowID       types.String `tfsdk:"workflow_id"`
	WorkflowType     types.String `tfsdk:"workflow_type"`
	TaskQueue        types.String `tfsdk:"task_queue"`
	Input            types.String `tfsdk:"input"`
	ExecutionTimeout types.String `tfsdk:"execution_timeout"`
	RunTimeout       types.String `tfsdk:"run_timeout"`
	TaskTimeout      types.String `tfsdk:"task_timeout"`
}

// ScheduleStateModel defines the state of a schedule.
type ScheduleStateModel struct {
	Paused           types.Bool   `tfsdk:"paused"`
	LimitedActions   types.Bool   `tfsdk:"limited_actions"`
	RemainingActions types.Int64  `tfsdk:"remaining_actions"`
	Notes            types.String `tfsdk:"notes"`
}

// SchedulePolicyModel defines the policy configuration for a schedule.
type SchedulePolicyModel struct {
	Overlap        types.String `tfsdk:"overlap_policy"`
	CatchupWindow  types.String `tfsdk:"catchup_window"`
	PauseOnFailure types.Bool   `tfsdk:"pause_on_failure"`
}

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
		MarkdownDescription: "Temporal Schedule resource for managing workflow schedules",
		Attributes: map[string]schema.Attribute{
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Namespace where the schedule resides",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schedule_id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for the schedule",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"memo": schema.MapAttribute{
				MarkdownDescription: "Non-indexed key-value pairs for metadata",
				ElementType:         types.StringType,
				Optional:            true,
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"spec": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule specification",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"intervals": schema.ListNestedAttribute{
						MarkdownDescription: "Time intervals for schedule",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"every": schema.StringAttribute{
									MarkdownDescription: "Duration of the interval (e.g., '24h', '7d')",
									Required:            true,
								},
								"offset": schema.StringAttribute{
									MarkdownDescription: "Offset from the interval (e.g., '1h')",
									Optional:            true,
								},
							},
						},
					},
					"calendar_items": schema.ListNestedAttribute{
						MarkdownDescription: "Calendar expressions for schedule",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"year": schema.StringAttribute{
									MarkdownDescription: "Year specification (e.g., '2022', '2022-2025')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("*"),
								},
								"month": schema.StringAttribute{
									MarkdownDescription: "Month specification in numeric format (e.g., '1', '1,2,9', '1-12')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("*"),
								},
								"day_of_month": schema.StringAttribute{
									MarkdownDescription: "Day of month specification (e.g., '1', '1,15', '1-31')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("*"),
								},
								"day_of_week": schema.StringAttribute{
									MarkdownDescription: "Day of week specification in numeric format (e.g., '1', '1-6', '1,3,5')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("0-6"),
								},
								"hour": schema.StringAttribute{
									MarkdownDescription: "Hour specification (e.g., '9', '9-17', '11-14')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("0"),
								},
								"minute": schema.StringAttribute{
									MarkdownDescription: "Minute specification (e.g., '0', '0,30', '*/15', '*')",
									Optional:            true,
									Computed:            true,
									Default:             stringdefault.StaticString("0"),
								},
								"second": schema.StringAttribute{
									MarkdownDescription: "Second specification (e.g., '0', '0,30', '*')",
									Computed:            true,
									Optional:            true,
									Default:             stringdefault.StaticString("0"),
								},
								"comment": schema.StringAttribute{
									MarkdownDescription: "Optional comment describing this calendar entry",
									Computed:            true,
									Optional:            true,
									Default:             stringdefault.StaticString(""),
								},
							},
						},
					},
					"cron_items": schema.ListAttribute{
						MarkdownDescription: "Traditional cron expressions (e.g. '15 8 * * *')",
						ElementType:         types.StringType,
						Optional:            true,
					},

					"start_time": schema.StringAttribute{
						MarkdownDescription: "Start time of the schedule (RFC3339)",
						Optional:            true,
					},
					"end_time": schema.StringAttribute{
						MarkdownDescription: "End time of the schedule (RFC3339)",
						Optional:            true,
					},
					"jitter": schema.StringAttribute{
						MarkdownDescription: "Jitter duration to add randomness to scheduled times",
						Optional:            true,
					},
					"time_zone": schema.StringAttribute{
						MarkdownDescription: "Time zone for the schedule",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("UTC"),
					},
				},
			},
			"action": schema.SingleNestedAttribute{
				MarkdownDescription: "Action to execute on schedule",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"workflow": schema.SingleNestedAttribute{
						MarkdownDescription: "Workflow action",
						Required:            true,
						Attributes: map[string]schema.Attribute{
							"workflow_id": schema.StringAttribute{
								MarkdownDescription: "Workflow ID",
								Required:            true,
							},
							"workflow_type": schema.StringAttribute{
								MarkdownDescription: "Workflow Type",
								Required:            true,
							},
							"task_queue": schema.StringAttribute{
								MarkdownDescription: "Task Queue",
								Required:            true,
							},
							"input": schema.StringAttribute{
								MarkdownDescription: "Workflow input (JSON)",
								Optional:            true,
							},
							"execution_timeout": schema.StringAttribute{
								MarkdownDescription: "Execution timeout",
								Optional:            true,
							},
							"run_timeout": schema.StringAttribute{
								MarkdownDescription: "Run timeout",
								Optional:            true,
							},
							"task_timeout": schema.StringAttribute{
								MarkdownDescription: "Task timeout",
								Optional:            true,
							},
						},
					},
				},
			},
			"state": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule state",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"paused": schema.BoolAttribute{
						MarkdownDescription: "Pause the Schedule immediately on creation",
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
					},
					"limited_actions": schema.BoolAttribute{
						MarkdownDescription: "Whether the schedule is limited to a specific number of actions",
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
					},
					"remaining_actions": schema.Int64Attribute{
						MarkdownDescription: "Total allowed actions",
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(0),
					},
					"notes": schema.StringAttribute{
						MarkdownDescription: "Initial notes",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString(""),
					},
				},
			},
			"policy_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule policy configuration",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"overlap_policy": schema.StringAttribute{
						MarkdownDescription: "Policy for handling overlapping Workflow Executions. Accepted values: Skip, BufferOne, BufferAll, CancelOther, TerminateOther, AllowAll",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("Skip"),
						Validators: []validator.String{
							stringvalidator.OneOf(
								"Skip",
								"BufferOne",
								"BufferAll",
								"CancelOther",
								"TerminateOther",
								"AllowAll",
							),
						},
					},
					"catchup_window": schema.StringAttribute{
						MarkdownDescription: "Maximum catch-up time for when the Service is unavailable",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString(defaultCatchupWindow),
					},
					"pause_on_failure": schema.BoolAttribute{
						MarkdownDescription: "Pause the schedule on action failure",
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
					},
				},
			},
		},
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
	var data ScheduleResourceModel

	client := workflowservice.NewWorkflowServiceClient(r.client)

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	scheduleSpec, diags := convertToScheduleSpec(data.Spec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	scheduleAction, diags := convertToScheduleAction(data.Action)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.GenerateUUID()
	if err != nil {
		resp.Diagnostics.AddError("UUID Generation Error", err.Error())
		return
	}

	memo, diags := convertToMemo(ctx, data.Memo)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	policy, diags := convertToSchedulePolicy(data.Policy)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := &workflowservice.CreateScheduleRequest{
		RequestId:  u,
		Namespace:  data.Namespace.ValueString(),
		ScheduleId: data.ScheduleID.ValueString(),
		Memo:       memo,
		Schedule: &schedulev1.Schedule{
			Spec:   scheduleSpec,
			Action: scheduleAction,
			State: &schedulev1.ScheduleState{
				Paused:           data.State.Paused.ValueBool(),
				LimitedActions:   data.State.LimitedActions.ValueBool(),
				RemainingActions: data.State.RemainingActions.ValueInt64(),
				Notes:            data.State.Notes.ValueString(),
			},
			// Policies: &schedulev1.SchedulePolicies{},
			Policies: policy,
		},
	}

	_, err = client.CreateSchedule(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.AlreadyExists); ok {
			resp.Diagnostics.AddError(
				"Schedule Already Exists",
				fmt.Sprintf("A schedule with ID %s already exists in namespace %s: %s",
					data.ScheduleID.ValueString(), data.Namespace.ValueString(), err.Error()),
			)
			return
		}
		resp.Diagnostics.AddError("Request error", "Schedule creation failed: "+err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Created schedule: %s in namespace: %s", data.ScheduleID.ValueString(), data.Namespace.ValueString()))

}

// Read reads the current state of a schedule.
func (r *ScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ScheduleResourceModel
	var diags diag.Diagnostics

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := workflowservice.NewWorkflowServiceClient(r.client)

	describeReq := &workflowservice.DescribeScheduleRequest{
		Namespace:  data.Namespace.ValueString(),
		ScheduleId: data.ScheduleID.ValueString(),
	}

	describeResp, err := client.DescribeSchedule(ctx, describeReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Schedule",
			fmt.Sprintf("Could not read schedule %s: %s", data.ScheduleID.ValueString(), err.Error()),
		)
		return
	}
	if describeResp.Memo != nil {
		data.Memo, diags = convertMemo(ctx, describeResp.GetMemo())
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if describeResp.Schedule != nil {
		if describeResp.Schedule.Spec != nil {
			data.Spec = convertScheduleSpec(describeResp.Schedule.Spec)
		}

		if describeResp.Schedule.Action != nil {
			data.Action = convertScheduleAction(describeResp.Schedule.Action)
		}
	}
	data.State = &ScheduleStateModel{
		Paused:           types.BoolValue(describeResp.Schedule.State.GetPaused()),
		LimitedActions:   types.BoolValue(describeResp.Schedule.State.GetLimitedActions()),
		RemainingActions: types.Int64Value(describeResp.Schedule.State.GetRemainingActions()),
		Notes:            types.StringValue(describeResp.Schedule.State.GetNotes()),
	}
	data.Policy = &SchedulePolicyModel{
		Overlap:        types.StringValue(describeResp.Schedule.Policies.OverlapPolicy.String()),
		CatchupWindow:  types.StringValue(formatDurationCanonical(describeResp.Schedule.Policies.CatchupWindow)),
		PauseOnFailure: types.BoolValue(describeResp.Schedule.Policies.GetPauseOnFailure()),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

}

// Update updates an existing schedule.
func (r *ScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ScheduleResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := workflowservice.NewWorkflowServiceClient(r.client)

	scheduleSpec, diags := convertToScheduleSpec(data.Spec)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	scheduleAction, diags := convertToScheduleAction(data.Action)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.GenerateUUID()
	if err != nil {
		resp.Diagnostics.AddError("UUID Generation Error", err.Error())
		return
	}

	policy, diags := convertToSchedulePolicy(data.Policy)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := &workflowservice.UpdateScheduleRequest{
		RequestId:  u,
		Namespace:  data.Namespace.ValueString(),
		ScheduleId: data.ScheduleID.ValueString(),
		Schedule: &schedulev1.Schedule{
			Spec:     scheduleSpec,
			Action:   scheduleAction,
			Policies: policy,
			State: &schedulev1.ScheduleState{
				Paused:           data.State.Paused.ValueBool(),
				LimitedActions:   data.State.LimitedActions.ValueBool(),
				RemainingActions: data.State.RemainingActions.ValueInt64(),
				Notes:            data.State.Notes.ValueString(),
			},
		},
	}

	_, err = client.UpdateSchedule(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Schedule",
			fmt.Sprintf("Could not update schedule %s: %s", data.ScheduleID.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	tflog.Info(ctx, fmt.Sprintf("Updated schedule: %s in namespace: %s",
		data.ScheduleID.ValueString(), data.Namespace.ValueString()))
}

// Delete deletes a schedule.
func (r *ScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ScheduleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := workflowservice.NewWorkflowServiceClient(r.client)

	deleteReq := &workflowservice.DeleteScheduleRequest{
		Namespace:  data.Namespace.ValueString(),
		ScheduleId: data.ScheduleID.ValueString(),
	}

	_, err := client.DeleteSchedule(ctx, deleteReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Schedule",
			fmt.Sprintf("Could not delete schedule %s: %s", data.ScheduleID.ValueString(), err.Error()),
		)
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Deleted schedule: %s in namespace: %s",
		data.ScheduleID.ValueString(), data.Namespace.ValueString()))
}

// ImportState imports an existing schedule into Terraform state.
func (r *ScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected request ID format is either 'namespace:schedule_id' or 'schedule_id'
	// Ex: 'default:example_schedule' or 'example_schedule'
	// If no namespace is provided, 'default' will be used

	var namespace, schedule string

	idTokens := strings.Split(req.ID, ":")
	switch len(idTokens) {
	case 1:
		// One part: schedule ID with default namespace ('example_schedule')
		namespace = "default"
		schedule = idTokens[0]
	case 2:
		// Two parts: the first is namespace, second is schedule ID ('default:schedule_example')
		namespace = idTokens[0]
		schedule = idTokens[1]
	default:
		// If neither, return an error
		resp.Diagnostics.AddError("Invalid ID format", "Expected 'namespace:schedule_id' or just 'schedule_id'.")
		return
	}

	// Validate the imported resource exists
	client := workflowservice.NewWorkflowServiceClient(r.client)
	_, err := client.DescribeSchedule(ctx, &workflowservice.DescribeScheduleRequest{
		Namespace:  namespace,
		ScheduleId: schedule,
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Failed",
			fmt.Sprintf("Schedule %s in namespace %s not found: %s", schedule, namespace, err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), namespace)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schedule_id"), schedule)...)
}

// Helper funcs.
func convertToScheduleAction(actionModel *ScheduleActionModel) (*schedulev1.ScheduleAction, diag.Diagnostics) {
	var diags diag.Diagnostics

	if actionModel == nil || actionModel.Workflow == nil {
		return nil, diags
	}

	action := &schedulev1.ScheduleAction{}

	workflowType := &commonv1.WorkflowType{
		Name: actionModel.Workflow.WorkflowType.ValueString(),
	}

	taskQueue := &taskqueuev1.TaskQueue{
		Name: actionModel.Workflow.TaskQueue.ValueString(),
	}

	workflowAction := &workflowv1.NewWorkflowExecutionInfo{
		WorkflowId:   actionModel.Workflow.WorkflowID.ValueString(),
		WorkflowType: workflowType,
		TaskQueue:    taskQueue,
	}

	if !actionModel.Workflow.Input.IsNull() {
		workflowAction.Input = &commonv1.Payloads{
			Payloads: []*commonv1.Payload{
				{
					Data: []byte(actionModel.Workflow.Input.ValueString()),
				},
			},
		}
	}

	if !actionModel.Workflow.ExecutionTimeout.IsNull() {
		executionTimeout, err := time.ParseDuration(actionModel.Workflow.ExecutionTimeout.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid Execution Timeout",
				fmt.Sprintf("Unable to parse execution timeout: %s. Error: %s",
					actionModel.Workflow.ExecutionTimeout.ValueString(), err),
			)
		}
		workflowAction.WorkflowExecutionTimeout = durationpb.New(executionTimeout)
	}

	if !actionModel.Workflow.RunTimeout.IsNull() {
		runTimeout, err := time.ParseDuration(actionModel.Workflow.RunTimeout.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid Run Timeout",
				fmt.Sprintf("Unable to parse run timeout: %s. Error: %s",
					actionModel.Workflow.RunTimeout.ValueString(), err),
			)
		}
		workflowAction.WorkflowRunTimeout = durationpb.New(runTimeout)
	}

	if !actionModel.Workflow.TaskTimeout.IsNull() {
		taskTimeout, err := time.ParseDuration(actionModel.Workflow.TaskTimeout.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid Task Timeout",
				fmt.Sprintf("Unable to parse task timeout: %s. Error: %s",
					actionModel.Workflow.TaskTimeout.ValueString(), err),
			)
		}
		workflowAction.WorkflowTaskTimeout = durationpb.New(taskTimeout)
	}

	action.Action = &schedulev1.ScheduleAction_StartWorkflow{
		StartWorkflow: workflowAction,
	}

	return action, diags
}

func convertToScheduleSpec(specModel *ScheduleSpecModel) (*schedulev1.ScheduleSpec, diag.Diagnostics) {
	if specModel == nil {
		return nil, nil
	}

	var diags diag.Diagnostics
	spec := &schedulev1.ScheduleSpec{}

	// Convert intervals
	if len(specModel.Intervals) > 0 {
		spec.Interval = make([]*schedulev1.IntervalSpec, 0, len(specModel.Intervals))
		for _, interval := range specModel.Intervals {
			intervalSpec := &schedulev1.IntervalSpec{}
			if !interval.Every.IsNull() {
				duration, err := time.ParseDuration(interval.Every.ValueString())
				if err != nil {
					diags.AddError(
						"Invalid Interval Duration",
						fmt.Sprintf("Unable to parse interval duration: %s. Error: %s", interval.Every.ValueString(), err),
					)
				} else {
					intervalSpec.Interval = durationpb.New(duration)
				}
			}
			if !interval.Offset.IsNull() {
				offset, err := time.ParseDuration(interval.Offset.ValueString())
				if err != nil {
					diags.AddError(
						"Invalid Interval Offset",
						fmt.Sprintf("Unable to parse interval offset: %s. Error: %s", interval.Offset.ValueString(), err),
					)
				} else {
					intervalSpec.Phase = durationpb.New(offset)
				}
			}
			spec.Interval = append(spec.Interval, intervalSpec)
		}
	}

	// Convert Calendar specifications
	if len(specModel.CalendarItems) > 0 {
		spec.Calendar = make([]*schedulev1.CalendarSpec, 0, len(specModel.CalendarItems))
		for _, e := range specModel.CalendarItems {
			cal := &schedulev1.CalendarSpec{
				Second:     e.Second.ValueString(),
				Minute:     e.Minute.ValueString(),
				Hour:       e.Hour.ValueString(),
				DayOfWeek:  e.DayOfWeek.ValueString(),
				DayOfMonth: e.DayOfMonth.ValueString(),
				Month:      e.Month.ValueString(),
				Year:       e.Year.ValueString(),
				Comment:    e.Comment.ValueString(),
			}
			spec.Calendar = append(spec.Calendar, cal)
		}
	}

	// Set cron expression if provided
	if len(specModel.CronItems) > 0 {
		spec.CronString = make([]string, 0, len(specModel.CronItems))
		for _, e := range specModel.CronItems {
			spec.CronString = append(spec.CronString, e.ValueString())
		}
	}

	// Set start time if provided
	if !specModel.StartTime.IsNull() {
		startTime, err := time.Parse(time.RFC3339, specModel.StartTime.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid Start Time",
				fmt.Sprintf("Unable to parse start time: %s. Error: %s", specModel.StartTime.ValueString(), err),
			)
		} else {
			spec.StartTime = timestamppb.New(startTime)
		}
	}

	// Set end time if provided
	if !specModel.EndTime.IsNull() {
		endTime, err := time.Parse(time.RFC3339, specModel.EndTime.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid End Time",
				fmt.Sprintf("Unable to parse end time: %s. Error: %s", specModel.EndTime.ValueString(), err),
			)
		} else {
			spec.EndTime = timestamppb.New(endTime)
		}
	}

	// Set jitter if provided
	if !specModel.Jitter.IsNull() {
		jitter, err := time.ParseDuration(specModel.Jitter.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid Jitter",
				fmt.Sprintf("Unable to parse jitter duration: %s. Error: %s", specModel.Jitter.ValueString(), err),
			)
		} else {
			spec.Jitter = durationpb.New(jitter)
		}
	}

	// Set timezone if provided
	if !specModel.TimeZone.IsNull() {
		spec.TimezoneName = specModel.TimeZone.ValueString()
	}

	return spec, diags
}

func ConvertFromTemporalSchedule(scheduleID string, namespace string, schedule *schedulev1.Schedule) (*ScheduleResourceModel, error) {
	if schedule == nil {
		return nil, fmt.Errorf("schedule description is nil")
	}
	model := &ScheduleResourceModel{
		Namespace:  types.StringValue(namespace),
		ScheduleID: types.StringValue(scheduleID),
	}

	if schedule.Spec != nil {
		model.Spec = convertScheduleSpec(schedule.Spec)
	}

	if schedule.Action != nil {
		model.Action = convertScheduleAction(schedule.Action)
	}

	return model, nil
}

// convertScheduleSpec converts Temporal ScheduleSpec to Terraform model.
func convertScheduleSpec(spec *schedulev1.ScheduleSpec) *ScheduleSpecModel {
	if spec == nil {
		return nil
	}

	tfSpec := &ScheduleSpecModel{
		TimeZone: types.StringValue(spec.TimezoneName),
	}

	// Convert cron expressions
	if len(spec.CronString) > 0 {
		tfSpec.CronItems = make([]types.String, 0, len(spec.CronString))
		for _, cron := range spec.CronString {
			tfSpec.CronItems = append(tfSpec.CronItems, types.StringValue(cron))
		}
	}

	// Convert calendar specs
	if len(spec.StructuredCalendar) > 0 {
		tfSpec.CalendarItems = make([]CalendarModel, 0, len(spec.Calendar))
		for _, calendar := range spec.StructuredCalendar {
			tfCalendar := CalendarModel{
				Year:       types.StringValue(formatRanges(calendar.Year)),
				Month:      types.StringValue(formatRanges(calendar.Month)),
				DayOfMonth: types.StringValue(formatRanges(calendar.DayOfMonth)),
				DayOfWeek:  types.StringValue(formatRanges(calendar.DayOfWeek)),
				Hour:       types.StringValue(formatRanges(calendar.Hour)),
				Minute:     types.StringValue(formatRanges(calendar.Minute)),
				Second:     types.StringValue(formatRanges(calendar.Second)),
				Comment:    types.StringValue(calendar.Comment),
			}
			tfSpec.CalendarItems = append(tfSpec.CalendarItems, tfCalendar)
		}
	}

	// If calendar items are being converted to cron, log it
	if len(spec.Calendar) == 0 && len(spec.CronString) > 0 {
		tflog.Warn(context.Background(), "Calendar items may have been converted to cron strings by Temporal")
	}
	if len(spec.Interval) > 0 {
		tfSpec.Intervals = make([]IntervalModel, 0, len(spec.Interval))
		for _, interval := range spec.Interval {
			tfInterval := IntervalModel{
				Every: types.StringValue(formatDurationCanonical(interval.Interval)),
			}
			if interval.Phase != nil {
				tfInterval.Offset = types.StringValue(formatDurationCanonical(interval.Phase))
			}
			tfSpec.Intervals = append(tfSpec.Intervals, tfInterval)
		}
	}

	if spec.StartTime != nil {
		tfSpec.StartTime = types.StringValue(spec.StartTime.AsTime().Format(time.RFC3339Nano))
	}
	if spec.EndTime != nil {
		tfSpec.EndTime = types.StringValue(spec.EndTime.AsTime().Format(time.RFC3339Nano))
	}

	if spec.Jitter != nil {
		tfSpec.Jitter = types.StringValue(spec.Jitter.String())
	}

	return tfSpec
}

// convertScheduleAction converts Temporal ScheduleAction to Terraform model.
func convertScheduleAction(action *schedulev1.ScheduleAction) *ScheduleActionModel {
	if action == nil {
		return nil
	}

	tfAction := &ScheduleActionModel{}
	workflowAction := action.GetStartWorkflow()
	if workflowAction != nil {
		tfWorkflow := &WorkflowActionModel{
			WorkflowID: types.StringValue(workflowAction.WorkflowId),
			TaskQueue:  types.StringValue(workflowAction.TaskQueue.Name),
		}

		if workflowAction.WorkflowType != nil {
			tfWorkflow.WorkflowType = types.StringValue(workflowAction.WorkflowType.Name)
		}

		if workflowAction.Input != nil && len(workflowAction.Input.Payloads) > 0 {
			tfWorkflow.Input = types.StringValue(string(workflowAction.Input.Payloads[0].Data))
		} else {
			tfWorkflow.Input = types.StringNull()
		}

		if workflowAction.WorkflowExecutionTimeout != nil {
			tfWorkflow.ExecutionTimeout = types.StringValue(formatDurationCanonical(workflowAction.WorkflowExecutionTimeout))
		}
		if workflowAction.WorkflowRunTimeout != nil {
			tfWorkflow.RunTimeout = types.StringValue(formatDurationCanonical(workflowAction.WorkflowRunTimeout))
		}
		if workflowAction.WorkflowTaskTimeout != nil {
			tfWorkflow.TaskTimeout = types.StringValue(formatDurationCanonical(workflowAction.WorkflowTaskTimeout))
		}

		tfAction.Workflow = tfWorkflow
	}

	return tfAction
}

func convertMemo(ctx context.Context, memo *commonv1.Memo) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics
	if memo == nil || len(memo.Fields) == 0 {
		return basetypes.MapValue{}, nil
	}

	data := make(map[string]string)

	for key, payload := range memo.Fields {
		var value string

		if err := json.Unmarshal(payload.Data, &value); err != nil {
			diags.AddError(fmt.Sprintf("Failed to unmarshal payload: %s", string(payload.GetData())), err.Error())
		}
		data[key] = value
	}
	result, diags := types.MapValueFrom(ctx, types.StringType, data)
	if diags.HasError() {
		return basetypes.MapValue{}, diags
	}

	return result, diags
}

func convertToMemo(ctx context.Context, data types.Map) (*commonv1.Memo, diag.Diagnostics) {
	var diags diag.Diagnostics

	elements := make(map[string]string)
	diags.Append(data.ElementsAs(ctx, &elements, false)...)

	memo := &commonv1.Memo{
		Fields: make(map[string]*commonv1.Payload),
	}

	for k, v := range elements {
		payload, err := createPayload(v)
		if err != nil {
			diags.AddError(fmt.Sprintf("failed to create payload for key: %s", k), err.Error())
			return nil, diags
		}
		memo.Fields[k] = payload
	}

	return memo, diags
}

// convertToSchedulePolicy converts a SchedulePolicyModel to a Temporal API SchedulePolicies.
func convertToSchedulePolicy(policyModel *SchedulePolicyModel) (*schedulev1.SchedulePolicies, diag.Diagnostics) {
	var diags diag.Diagnostics

	policies := &schedulev1.SchedulePolicies{}
	// Set overlap policy
	overlap, ok := ScheduleOverlapPolicy[policyModel.Overlap.ValueString()]
	if !ok {
		diags.AddError(
			"Invalid Overlap Policy",
			fmt.Sprintf("Invalid overlap policy value: %s. Allowed values are SCHEDULE_OVERLAP_POLICY_SKIP, SCHEDULE_OVERLAP_POLICY_BUFFER_ONE, SCHEDULE_OVERLAP_POLICY_BUFFER_ALL, SCHEDULE_OVERLAP_POLICY_CANCEL_OTHER, or SCHEDULE_OVERLAP_POLICY_TERMINATE_OTHER.", policyModel.Overlap.ValueString()),
		)
		return nil, diags

	}
	policies.OverlapPolicy = overlap

	// Set catchup window
	duration, err := time.ParseDuration(policyModel.CatchupWindow.ValueString())
	if err != nil {
		diags.AddError(
			"Invalid Catchup Window",
			fmt.Sprintf("Unable to parse catchup window: %s. Error: %s", policyModel.CatchupWindow.ValueString(), err),
		)
		return nil, diags
	}
	policies.CatchupWindow = durationpb.New(duration)

	// Set pause on failure
	policies.PauseOnFailure = policyModel.PauseOnFailure.ValueBool()

	return policies, diags
}
