package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	commonv1 "go.temporal.io/api/common/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

var (
	_ datasource.DataSource              = &ScheduleDataSource{}
	_ datasource.DataSourceWithConfigure = &ScheduleDataSource{}
)

func NewScheduleDataSource() datasource.DataSource {
	return &ScheduleDataSource{}
}

type ScheduleDataSource struct {
	client workflowservice.WorkflowServiceClient
}

func (d *ScheduleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule"
}

func (d *ScheduleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Temporal Schedule data source",
		Attributes: map[string]schema.Attribute{
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Namespace where the schedule resides",
				Required:            true,
			},
			"schedule_id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for the schedule",
				Required:            true,
			},
			"memo": schema.MapAttribute{
				MarkdownDescription: "Non-indexed key-value pairs for metadata. Values that are not JSON strings are returned as their JSON text, binary values are base64-encoded",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"spec": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule specification",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"intervals": schema.ListNestedAttribute{
						MarkdownDescription: "Time intervals for schedule. Empty when the schedule has none",
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"every": schema.StringAttribute{
									MarkdownDescription: "Duration of the interval (e.g., '24h')",
									Computed:            true,
								},
								"offset": schema.StringAttribute{
									MarkdownDescription: "Offset from the interval (e.g., '1h')",
									Computed:            true,
								},
							},
						},
					},
					"calendar_items": schema.ListNestedAttribute{
						MarkdownDescription: "Calendar expressions for schedule. Temporal compiles cron expressions into calendar expressions, so they are returned here as well. Empty when the schedule has none",
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"year": schema.StringAttribute{
									MarkdownDescription: "Year specification (e.g., '2022', '2022-2025'). Empty when every year matches",
									Computed:            true,
								},
								"month": schema.StringAttribute{
									MarkdownDescription: "Month specification in numeric format (e.g., '1', '1,2,9', '1-12')",
									Computed:            true,
								},
								"day_of_month": schema.StringAttribute{
									MarkdownDescription: "Day of month specification (e.g., '1', '1,15', '1-31')",
									Computed:            true,
								},
								"day_of_week": schema.StringAttribute{
									MarkdownDescription: "Day of week specification in numeric format (e.g., '1', '1-6', '1,3,5')",
									Computed:            true,
								},
								"hour": schema.StringAttribute{
									MarkdownDescription: "Hour specification (e.g., '9', '9-17', '11-14')",
									Computed:            true,
								},
								"minute": schema.StringAttribute{
									MarkdownDescription: "Minute specification (e.g., '0', '0,30')",
									Computed:            true,
								},
								"second": schema.StringAttribute{
									MarkdownDescription: "Second specification (e.g., '0', '0,30')",
									Computed:            true,
								},
								"comment": schema.StringAttribute{
									MarkdownDescription: "Comment describing this calendar entry",
									Computed:            true,
								},
							},
						},
					},
					"cron_items": schema.ListAttribute{
						MarkdownDescription: "Traditional cron expressions. Temporal compiles them into `calendar_items`, so this is normally empty",
						ElementType:         types.StringType,
						Computed:            true,
					},
					"start_time": schema.StringAttribute{
						MarkdownDescription: "Start time of the schedule (RFC3339)",
						Computed:            true,
					},
					"end_time": schema.StringAttribute{
						MarkdownDescription: "End time of the schedule (RFC3339)",
						Computed:            true,
					},
					"jitter": schema.StringAttribute{
						MarkdownDescription: "Jitter duration to add randomness to scheduled times",
						Computed:            true,
					},
					"time_zone": schema.StringAttribute{
						MarkdownDescription: "Time zone for the schedule. Empty when the schedule uses the server default (UTC)",
						Computed:            true,
					},
				},
			},
			"action": schema.SingleNestedAttribute{
				MarkdownDescription: "Action to execute on schedule",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"workflow": schema.SingleNestedAttribute{
						MarkdownDescription: "Workflow action",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"workflow_id": schema.StringAttribute{
								MarkdownDescription: "Workflow ID",
								Computed:            true,
							},
							"workflow_type": schema.StringAttribute{
								MarkdownDescription: "Workflow Type",
								Computed:            true,
							},
							"task_queue": schema.StringAttribute{
								MarkdownDescription: "Task Queue",
								Computed:            true,
							},
							"input": schema.StringAttribute{
								MarkdownDescription: "Workflow input (JSON). Only the first argument is returned when the workflow is started with several, and input that is not text is base64-encoded. Both cases are reported with a warning",
								Computed:            true,
							},
							"execution_timeout": schema.StringAttribute{
								MarkdownDescription: "Execution timeout",
								Computed:            true,
							},
							"run_timeout": schema.StringAttribute{
								MarkdownDescription: "Run timeout",
								Computed:            true,
							},
							"task_timeout": schema.StringAttribute{
								MarkdownDescription: "Task timeout",
								Computed:            true,
							},
						},
					},
				},
			},
			"state": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule state",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"paused": schema.BoolAttribute{
						MarkdownDescription: "Whether the schedule is paused",
						Computed:            true,
					},
					"limited_actions": schema.BoolAttribute{
						MarkdownDescription: "Whether the schedule is limited to a specific number of actions",
						Computed:            true,
					},
					"remaining_actions": schema.Int64Attribute{
						MarkdownDescription: "Remaining allowed actions",
						Computed:            true,
					},
					"notes": schema.StringAttribute{
						MarkdownDescription: "Notes",
						Computed:            true,
					},
				},
			},
			"policy_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Schedule policy configuration",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"overlap_policy": schema.StringAttribute{
						MarkdownDescription: "Policy for handling overlapping Workflow Executions",
						Computed:            true,
					},
					"catchup_window": schema.StringAttribute{
						MarkdownDescription: "Maximum catch-up time for when the Service is unavailable",
						Computed:            true,
					},
					"pause_on_failure": schema.BoolAttribute{
						MarkdownDescription: "Pause the schedule on action failure",
						Computed:            true,
					},
				},
			},
		},
	}
}

func (d *ScheduleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.client = workflowservice.NewWorkflowServiceClient(connection)
}

func (d *ScheduleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ScheduleResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeResp, err := d.client.DescribeSchedule(ctx, &workflowservice.DescribeScheduleRequest{
		Namespace:  data.Namespace.ValueString(),
		ScheduleId: data.ScheduleID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Schedule",
			fmt.Sprintf("Could not read schedule %s in namespace %s: %s", data.ScheduleID.ValueString(), data.Namespace.ValueString(), err.Error()),
		)
		return
	}

	memo, diags := scheduleMemoToMap(ctx, describeResp.GetMemo())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	schedule := describeResp.GetSchedule()
	action, diags := convertScheduleAction(schedule.GetAction())
	resp.Diagnostics.Append(diags...)

	data.Memo = memo
	data.Spec = scheduleDataSourceSpec(schedule.GetSpec())
	data.Action = action
	data.State = &ScheduleStateModel{
		Paused:           types.BoolValue(schedule.GetState().GetPaused()),
		LimitedActions:   types.BoolValue(schedule.GetState().GetLimitedActions()),
		RemainingActions: types.Int64Value(schedule.GetState().GetRemainingActions()),
		Notes:            types.StringValue(schedule.GetState().GetNotes()),
	}
	data.Policy = &SchedulePolicyModel{
		Overlap:        types.StringValue(schedule.GetPolicies().GetOverlapPolicy().String()),
		CatchupWindow:  types.StringValue(formatDurationCanonical(schedule.GetPolicies().GetCatchupWindow())),
		PauseOnFailure: types.BoolValue(schedule.GetPolicies().GetPauseOnFailure()),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func scheduleDataSourceSpec(spec *schedulev1.ScheduleSpec) *ScheduleSpecModel {
	model := convertScheduleSpec(spec)
	if model == nil {
		return nil
	}

	if model.Intervals == nil {
		model.Intervals = []IntervalModel{}
	}
	if model.CalendarItems == nil {
		model.CalendarItems = []CalendarModel{}
	}
	if model.CronItems == nil {
		model.CronItems = []types.String{}
	}

	return model
}

func scheduleMemoToMap(ctx context.Context, memo *commonv1.Memo) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	values := make(map[string]string, len(memo.GetFields()))
	for key, payload := range memo.GetFields() {
		var value string
		if err := json.Unmarshal(payload.GetData(), &value); err != nil {
			text, binary := payloadText(payload)
			if binary {
				diags.AddWarning(
					"Binary Schedule Memo Value",
					fmt.Sprintf("Memo value %q is not text (it may be encrypted by a payload codec) and is returned base64-encoded.", key),
				)
			}
			value = text
		}
		values[key] = value
	}

	result, mapDiags := types.MapValueFrom(ctx, types.StringType, values)
	diags.Append(mapDiags...)

	return result, diags
}
