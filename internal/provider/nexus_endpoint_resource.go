package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	commonv1 "go.temporal.io/api/common/v1"
	nexusv1 "go.temporal.io/api/nexus/v1"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const nexusAPIsDisabledMessage = "Nexus APIs are disabled"

var (
	_ resource.Resource                     = &NexusEndpointResource{}
	_ resource.ResourceWithConfigure        = &NexusEndpointResource{}
	_ resource.ResourceWithConfigValidators = &NexusEndpointResource{}
	_ resource.ResourceWithImportState      = &NexusEndpointResource{}

	nexusEndpointNameRegex        = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-]*[a-zA-Z0-9]$`)
	nexusEndpointExternalURLRegex = regexp.MustCompile(`^https?://`)
)

func NewNexusEndpointResource() resource.Resource {
	return &NexusEndpointResource{}
}

type NexusEndpointResource struct {
	client operatorservice.OperatorServiceClient
}

type NexusEndpointModel struct {
	Id             types.String                      `tfsdk:"id"`
	Name           types.String                      `tfsdk:"name"`
	Description    types.String                      `tfsdk:"description"`
	WorkerTarget   *NexusEndpointWorkerTargetModel   `tfsdk:"worker_target"`
	ExternalTarget *NexusEndpointExternalTargetModel `tfsdk:"external_target"`
	Version        types.Int64                       `tfsdk:"version"`
	UrlPrefix      types.String                      `tfsdk:"url_prefix"`
}

type NexusEndpointWorkerTargetModel struct {
	Namespace types.String `tfsdk:"namespace"`
	TaskQueue types.String `tfsdk:"task_queue"`
}

type NexusEndpointExternalTargetModel struct {
	Url types.String `tfsdk:"url"`
}

func (r *NexusEndpointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nexus_endpoint"
}

func (r *NexusEndpointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Temporal Nexus Endpoint resource. An endpoint routes Nexus requests either to a task queue in a namespace or to an external URL. Exactly one of `worker_target` and `external_target` must be set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-generated endpoint identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Endpoint name, unique within the cluster. Must start with a letter, end with a letter or a digit, and contain only letters, digits and hyphens. The endpoint is renamed in place, which breaks callers that reference the old name",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(nexusEndpointNameRegex, "must start with a letter, end with a letter or a digit, and contain only letters, digits and hyphens"),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Endpoint description in Markdown",
				Optional:            true,
			},
			"worker_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Route requests to a task queue polled by Nexus workers",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"namespace": schema.StringAttribute{
						MarkdownDescription: "Namespace to route requests to. It must exist, and it cannot be deleted while the endpoint targets it",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"task_queue": schema.StringAttribute{
						MarkdownDescription: "Nexus task queue to route requests to",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
				},
			},
			"external_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Route requests to an external Nexus server",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						MarkdownDescription: "URL to call, with the `http` or `https` scheme",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(nexusEndpointExternalURLRegex, "must use the http or https scheme"),
						},
					},
				},
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Data version of the endpoint, incremented by the server on every update. Updates fail when the endpoint was changed outside Terraform after the last refresh",
				Computed:            true,
			},
			"url_prefix": schema.StringAttribute{
				MarkdownDescription: "Server URL path prefix for invoking operations on this endpoint. It does not change when the endpoint is renamed",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *NexusEndpointResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("worker_target"),
			path.MatchRoot("external_target"),
		),
	}
}

func (r *NexusEndpointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.client = operatorservice.NewOperatorServiceClient(connection)
}

func (r *NexusEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NexusEndpointModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createResp, err := r.client.CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{
		Spec: data.endpointSpec(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Nexus Endpoint",
			fmt.Sprintf("Could not create Nexus endpoint %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(data.setFromEndpoint(createResp.GetEndpoint())...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NexusEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NexusEndpointModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	getResp, err := r.client.GetNexusEndpoint(ctx, &operatorservice.GetNexusEndpointRequest{
		Id: data.Id.ValueString(),
	})
	if err != nil {
		if isNexusEndpointNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Nexus Endpoint",
			fmt.Sprintf("Could not read Nexus endpoint %s: %s", data.Id.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(data.setFromEndpoint(getResp.GetEndpoint())...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NexusEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state NexusEndpointModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateResp, err := r.client.UpdateNexusEndpoint(ctx, &operatorservice.UpdateNexusEndpointRequest{
		Id:      state.Id.ValueString(),
		Version: state.Version.ValueInt64(),
		Spec:    data.endpointSpec(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Nexus Endpoint",
			fmt.Sprintf("Could not update Nexus endpoint %s: %s", state.Id.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(data.setFromEndpoint(updateResp.GetEndpoint())...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NexusEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NexusEndpointModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.DeleteNexusEndpoint(ctx, &operatorservice.DeleteNexusEndpointRequest{
		Id:      data.Id.ValueString(),
		Version: data.Version.ValueInt64(),
	})
	if err != nil && !isNexusEndpointNotFound(err) {
		resp.Diagnostics.AddError(
			"Error Deleting Nexus Endpoint",
			fmt.Sprintf("Could not delete Nexus endpoint %s: %s", data.Id.ValueString(), err.Error()),
		)
	}
}

func (r *NexusEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	endpoint, err := findNexusEndpoint(ctx, r.client, req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Import Failed",
			fmt.Sprintf("Could not find Nexus endpoint %s: %s", req.ID, err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), endpoint.GetId())...)
}

func findNexusEndpoint(ctx context.Context, client operatorservice.OperatorServiceClient, idOrName string) (*nexusv1.Endpoint, error) {
	if id, ok := canonicalUUID(idOrName); ok {
		getResp, err := client.GetNexusEndpoint(ctx, &operatorservice.GetNexusEndpointRequest{Id: id})
		if err == nil {
			return getResp.GetEndpoint(), nil
		}
		if !isNexusEndpointNotFound(err) {
			return nil, err
		}
	}

	return findNexusEndpointByName(ctx, client, idOrName)
}

func findNexusEndpointByName(ctx context.Context, client operatorservice.OperatorServiceClient, name string) (*nexusv1.Endpoint, error) {
	listResp, err := client.ListNexusEndpoints(ctx, &operatorservice.ListNexusEndpointsRequest{Name: name})
	if err != nil {
		return nil, err
	}
	if len(listResp.GetEndpoints()) == 0 {
		return nil, fmt.Errorf("not found")
	}

	return listResp.GetEndpoints()[0], nil
}

func canonicalUUID(value string) (string, bool) {
	parsed, err := uuid.ParseUUID(value)
	if err != nil {
		return "", false
	}
	formatted, err := uuid.FormatUUID(parsed)
	if err != nil {
		return "", false
	}

	return formatted, true
}

func isNexusEndpointNotFound(err error) bool {
	st, ok := status.FromError(err)

	return ok && st.Code() == codes.NotFound && st.Message() != nexusAPIsDisabledMessage
}

func (m *NexusEndpointModel) endpointSpec() *nexusv1.EndpointSpec {
	spec := &nexusv1.EndpointSpec{
		Name:        m.Name.ValueString(),
		Description: nexusEndpointDescriptionPayload(m.Description),
	}

	switch {
	case m.WorkerTarget != nil:
		spec.Target = &nexusv1.EndpointTarget{Variant: &nexusv1.EndpointTarget_Worker_{
			Worker: &nexusv1.EndpointTarget_Worker{
				Namespace: m.WorkerTarget.Namespace.ValueString(),
				TaskQueue: m.WorkerTarget.TaskQueue.ValueString(),
			},
		}}
	case m.ExternalTarget != nil:
		spec.Target = &nexusv1.EndpointTarget{Variant: &nexusv1.EndpointTarget_External_{
			External: &nexusv1.EndpointTarget_External{
				Url: m.ExternalTarget.Url.ValueString(),
			},
		}}
	}

	return spec
}

func (m *NexusEndpointModel) setFromEndpoint(endpoint *nexusv1.Endpoint) diag.Diagnostics {
	spec := endpoint.GetSpec()
	description, diags := nexusEndpointDescription(spec.GetDescription())

	m.Id = types.StringValue(endpoint.GetId())
	m.Name = types.StringValue(spec.GetName())
	m.Description = description
	m.Version = types.Int64Value(endpoint.GetVersion())
	m.UrlPrefix = types.StringValue(endpoint.GetUrlPrefix())
	m.WorkerTarget = nil
	m.ExternalTarget = nil

	if worker := spec.GetTarget().GetWorker(); worker != nil {
		m.WorkerTarget = &NexusEndpointWorkerTargetModel{
			Namespace: types.StringValue(worker.GetNamespace()),
			TaskQueue: types.StringValue(worker.GetTaskQueue()),
		}
	}
	if external := spec.GetTarget().GetExternal(); external != nil {
		m.ExternalTarget = &NexusEndpointExternalTargetModel{
			Url: types.StringValue(external.GetUrl()),
		}
	}

	return diags
}

func nexusEndpointDescriptionPayload(description types.String) *commonv1.Payload {
	if description.IsNull() || description.IsUnknown() {
		return nil
	}
	data, _ := json.Marshal(description.ValueString())

	return &commonv1.Payload{
		Metadata: map[string][]byte{"encoding": []byte("json/plain")},
		Data:     data,
	}
}

func nexusEndpointDescription(payload *commonv1.Payload) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics

	if payload == nil {
		return types.StringNull(), diags
	}

	raw := payload.GetData()

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		if utf8.Valid(raw) {
			value = string(raw)
		} else {
			value = base64.StdEncoding.EncodeToString(raw)
			diags.AddWarning(
				"Binary Nexus Endpoint Description",
				"The endpoint description is not text (it may be encrypted by a payload codec) and is returned base64-encoded.",
			)
		}
	}

	return types.StringValue(value), diags
}
