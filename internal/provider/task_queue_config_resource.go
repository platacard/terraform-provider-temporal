package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"go.temporal.io/api/enums/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const taskQueueTypeWorkflow = "Workflow"

var (
	_ resource.Resource                   = &TaskQueueConfigResource{}
	_ resource.ResourceWithConfigure      = &TaskQueueConfigResource{}
	_ resource.ResourceWithValidateConfig = &TaskQueueConfigResource{}
	_ resource.ResourceWithImportState    = &TaskQueueConfigResource{}
)

func NewTaskQueueConfigResource() resource.Resource {
	return &TaskQueueConfigResource{}
}

type TaskQueueConfigResource struct {
	client workflowservice.WorkflowServiceClient
}

type TaskQueueConfigModel struct {
	Namespace                   types.String             `tfsdk:"namespace"`
	TaskQueue                   types.String             `tfsdk:"task_queue"`
	TaskQueueType               types.String             `tfsdk:"task_queue_type"`
	QueueRateLimit              *TaskQueueRateLimitModel `tfsdk:"queue_rate_limit"`
	FairnessKeyRateLimitDefault *TaskQueueRateLimitModel `tfsdk:"fairness_key_rate_limit_default"`
	FairnessWeightOverrides     types.Map                `tfsdk:"fairness_weight_overrides"`
}

type TaskQueueRateLimitModel struct {
	RequestsPerSecond types.Float64 `tfsdk:"requests_per_second"`
	Reason            types.String  `tfsdk:"reason"`
}

func (r *TaskQueueConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_task_queue_config"
}

func (r *TaskQueueConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	rateLimitAttributes := map[string]schema.Attribute{
		"requests_per_second": schema.Float64Attribute{
			MarkdownDescription: "Maximum number of tasks dispatched per second. Zero is a valid limit and stops dispatching. The server keeps about 7 significant digits",
			Required:            true,
			Validators: []validator.Float64{
				float64validator.AtLeast(0),
				float32PrecisionValidator{},
			},
		},
		"reason": schema.StringAttribute{
			MarkdownDescription: "Reason why the limit was set",
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(""),
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Temporal Task Queue configuration resource. Manages the rate limits and fairness weights of one task queue type. " +
			"The resource owns the whole configuration: settings that are not in the Terraform configuration are removed from the task queue, and destroying the resource resets it. " +
			"Requires Temporal Server 1.29 or later.",
		Attributes: map[string]schema.Attribute{
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Namespace of the task queue",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"task_queue": schema.StringAttribute{
				MarkdownDescription: "Task queue name. The task queue does not need to have workers yet",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"task_queue_type": schema.StringAttribute{
				MarkdownDescription: "Task queue type the configuration applies to. Accepted values: Workflow, Activity, Nexus. Rate limits cannot be set on Workflow task queues",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(taskQueueTypeWorkflow, "Activity", "Nexus"),
				},
			},
			"queue_rate_limit": schema.SingleNestedAttribute{
				MarkdownDescription: "Rate limit of the whole task queue. It overrides the limit set by workers",
				Optional:            true,
				Attributes:          rateLimitAttributes,
			},
			"fairness_key_rate_limit_default": schema.SingleNestedAttribute{
				MarkdownDescription: "Default rate limit of each fairness key, scaled by the weight of the key",
				Optional:            true,
				Attributes:          rateLimitAttributes,
			},
			"fairness_weight_overrides": schema.MapAttribute{
				MarkdownDescription: "Fairness weight for each fairness key. It takes precedence over the weight attached to a task. Values must be between 0.001 and 1000. The server keeps about 7 significant digits",
				ElementType:         types.Float64Type,
				Optional:            true,
				Computed:            true,
				Default:             mapdefault.StaticValue(types.MapValueMust(types.Float64Type, map[string]attr.Value{})),
				Validators: []validator.Map{
					mapvalidator.ValueFloat64sAre(float64validator.Between(0.001, 1000), float32PrecisionValidator{}),
				},
			},
		},
	}
}

func (r *TaskQueueConfigResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var taskQueueType types.String
	var queueRateLimit, fairnessKeyRateLimitDefault types.Object

	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("task_queue_type"), &taskQueueType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("queue_rate_limit"), &queueRateLimit)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("fairness_key_rate_limit_default"), &fairnessKeyRateLimitDefault)...)
	if resp.Diagnostics.HasError() || taskQueueType.ValueString() != taskQueueTypeWorkflow {
		return
	}

	for name, value := range map[string]types.Object{
		"queue_rate_limit":                queueRateLimit,
		"fairness_key_rate_limit_default": fairnessKeyRateLimitDefault,
	} {
		if !value.IsNull() && !value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Invalid Attribute Combination",
				"Rate limits cannot be set on Workflow task queues. Only fairness_weight_overrides is supported for this task queue type.",
			)
		}
	}
}

func (r *TaskQueueConfigResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = workflowservice.NewWorkflowServiceClient(connection)
}

func (r *TaskQueueConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TaskQueueConfigModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TaskQueueConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TaskQueueConfigModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, err := r.describe(ctx, &data)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Task Queue Configuration", data.errorMessage("read", err))
		return
	}

	resp.Diagnostics.Append(data.setFromConfig(ctx, config)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TaskQueueConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TaskQueueConfigModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TaskQueueConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TaskQueueConfigModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.QueueRateLimit = nil
	data.FairnessKeyRateLimitDefault = nil
	data.FairnessWeightOverrides = types.MapNull(types.Float64Type)

	config, err := r.describe(ctx, &data)
	if err == nil {
		_, err = r.client.UpdateTaskQueueConfig(ctx, data.updateRequest(nil, config))
	}
	if err != nil && status.Code(err) != codes.NotFound {
		resp.Diagnostics.AddError("Error Deleting Task Queue Configuration", data.errorMessage("reset", err))
	}
}

func (r *TaskQueueConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	namespace, taskQueue, taskQueueType, ok := parseTaskQueueConfigID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid ID format",
			"Expected 'namespace:task_queue:task_queue_type', where task_queue_type is Workflow, Activity or Nexus.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), namespace)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("task_queue"), taskQueue)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("task_queue_type"), taskQueueType)...)
}

func (r *TaskQueueConfigResource) apply(ctx context.Context, data *TaskQueueConfigModel) diag.Diagnostics {
	var diags diag.Diagnostics

	configured := map[string]float64{}
	diags.Append(data.FairnessWeightOverrides.ElementsAs(ctx, &configured, false)...)
	if diags.HasError() {
		return diags
	}
	weights := make(map[string]float32, len(configured))
	for key, weight := range configured {
		weights[key] = float32(weight)
	}

	current, err := r.describe(ctx, data)
	if err != nil {
		diags.AddError("Error Reading Task Queue Configuration", data.errorMessage("read", err))
		return diags
	}

	updateResp, err := r.client.UpdateTaskQueueConfig(ctx, data.updateRequest(weights, current))
	if err != nil {
		diags.AddError("Error Updating Task Queue Configuration", data.errorMessage("update", err))
		return diags
	}

	diags.Append(data.setFromConfig(ctx, updateResp.GetConfig())...)

	return diags
}

func (r *TaskQueueConfigResource) describe(ctx context.Context, data *TaskQueueConfigModel) (*taskqueuev1.TaskQueueConfig, error) {
	describeResp, err := r.client.DescribeTaskQueue(ctx, &workflowservice.DescribeTaskQueueRequest{
		Namespace:     data.Namespace.ValueString(),
		TaskQueue:     &taskqueuev1.TaskQueue{Name: data.TaskQueue.ValueString()},
		TaskQueueType: data.taskQueueType(),
		ReportConfig:  true,
	})
	if err != nil {
		return nil, err
	}

	return describeResp.GetConfig(), nil
}

func (m *TaskQueueConfigModel) taskQueueType() enums.TaskQueueType {
	taskQueueType, _ := enums.TaskQueueTypeFromString(m.TaskQueueType.ValueString())

	return taskQueueType
}

func (m *TaskQueueConfigModel) errorMessage(action string, err error) string {
	message := fmt.Sprintf("Could not %s the configuration of %s task queue %s in namespace %s: %s",
		action, m.TaskQueueType.ValueString(), m.TaskQueue.ValueString(), m.Namespace.ValueString(), err.Error())
	if status.Code(err) == codes.Unimplemented {
		message += "\n\nThe Temporal server does not support task queue configuration. It requires Temporal Server 1.29 or later."
	}

	return message
}

func (m *TaskQueueConfigModel) updateRequest(weights map[string]float32, current *taskqueuev1.TaskQueueConfig) *workflowservice.UpdateTaskQueueConfigRequest {
	request := &workflowservice.UpdateTaskQueueConfigRequest{
		Namespace:                         m.Namespace.ValueString(),
		TaskQueue:                         m.TaskQueue.ValueString(),
		TaskQueueType:                     m.taskQueueType(),
		UpdateQueueRateLimit:              taskQueueRateLimitUpdate(m.QueueRateLimit, current.GetQueueRateLimit()),
		UpdateFairnessKeyRateLimitDefault: taskQueueRateLimitUpdate(m.FairnessKeyRateLimitDefault, current.GetFairnessKeysRateLimitDefault()),
		SetFairnessWeightOverrides:        weights,
	}
	for key := range current.GetFairnessWeightOverrides() {
		if _, ok := weights[key]; !ok {
			request.UnsetFairnessWeightOverrides = append(request.UnsetFairnessWeightOverrides, key)
		}
	}

	return request
}

func (m *TaskQueueConfigModel) setFromConfig(ctx context.Context, config *taskqueuev1.TaskQueueConfig) diag.Diagnostics {
	weights := make(map[string]float64, len(config.GetFairnessWeightOverrides()))
	for key, weight := range config.GetFairnessWeightOverrides() {
		weights[key] = float32ToFloat64(weight)
	}
	overrides, diags := types.MapValueFrom(ctx, types.Float64Type, weights)

	m.QueueRateLimit = taskQueueRateLimitModel(config.GetQueueRateLimit())
	m.FairnessKeyRateLimitDefault = taskQueueRateLimitModel(config.GetFairnessKeysRateLimitDefault())
	m.FairnessWeightOverrides = overrides

	return diags
}

func taskQueueRateLimitUpdate(desired *TaskQueueRateLimitModel, current *taskqueuev1.RateLimitConfig) *workflowservice.UpdateTaskQueueConfigRequest_RateLimitUpdate {
	if desired != nil {
		return &workflowservice.UpdateTaskQueueConfigRequest_RateLimitUpdate{
			RateLimit: &taskqueuev1.RateLimit{RequestsPerSecond: float32(desired.RequestsPerSecond.ValueFloat64())},
			Reason:    desired.Reason.ValueString(),
		}
	}
	if current.GetRateLimit() != nil {
		return &workflowservice.UpdateTaskQueueConfigRequest_RateLimitUpdate{}
	}

	return nil
}

func taskQueueRateLimitModel(config *taskqueuev1.RateLimitConfig) *TaskQueueRateLimitModel {
	if config.GetRateLimit() == nil {
		return nil
	}

	return &TaskQueueRateLimitModel{
		RequestsPerSecond: types.Float64Value(float32ToFloat64(config.GetRateLimit().GetRequestsPerSecond())),
		Reason:            types.StringValue(config.GetMetadata().GetReason()),
	}
}

func parseTaskQueueConfigID(id string) (namespace, taskQueue, taskQueueType string, ok bool) {
	first := strings.Index(id, ":")
	last := strings.LastIndex(id, ":")
	if first <= 0 || last <= first+1 || last == len(id)-1 {
		return "", "", "", false
	}

	taskQueueType = id[last+1:]
	if taskQueueType != taskQueueTypeWorkflow && taskQueueType != "Activity" && taskQueueType != "Nexus" {
		return "", "", "", false
	}

	return id[:first], id[first+1 : last], taskQueueType, true
}

func float32ToFloat64(value float32) float64 {
	shortest, _ := strconv.ParseFloat(strconv.FormatFloat(float64(value), 'g', -1, 32), 64)

	return shortest
}

type float32PrecisionValidator struct{}

func (float32PrecisionValidator) Description(context.Context) string {
	return "value must be storable as a 32-bit float without rounding (about 7 significant digits)"
}

func (v float32PrecisionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (float32PrecisionValidator) ValidateFloat64(ctx context.Context, req validator.Float64Request, resp *validator.Float64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueFloat64()
	if stored := float32ToFloat64(float32(value)); stored != value {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Attribute Value Precision",
			fmt.Sprintf("The server stores this value as a 32-bit float, which keeps about 7 significant digits: %v would be stored as %v. Use %v instead.",
				strconv.FormatFloat(value, 'g', -1, 64), strconv.FormatFloat(stored, 'g', -1, 64), strconv.FormatFloat(stored, 'g', -1, 64)),
		)
	}
}
