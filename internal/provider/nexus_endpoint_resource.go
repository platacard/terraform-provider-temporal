package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	commonv1 "go.temporal.io/api/common/v1"
	nexusv1 "go.temporal.io/api/nexus/v1"
	"go.temporal.io/api/operatorservice/v1"
)

// Package-level attribute-type maps for the worker_target / external_target
// nested attributes. Defined once and reused at every callsite (encode, decode,
// data source) to keep the shape definition in one place.
var (
	workerTargetAttrTypes = map[string]attr.Type{
		"namespace":  types.StringType,
		"task_queue": types.StringType,
	}

	externalTargetAttrTypes = map[string]attr.Type{
		"url": types.StringType,
	}

	// externalTargetURLRegex matches an http(s):// URL. The Nexus server
	// dispatches external endpoints via HTTP, so allow both http and https
	// at schema-validation time and let server policy reject plaintext if
	// configured to. Caught at plan time rather than at apply.
	externalTargetURLRegex = regexp.MustCompile(`(?i)^https?://`)
)

var (
	_ resource.Resource                = &NexusEndpointResource{}
	_ resource.ResourceWithConfigure   = &NexusEndpointResource{}
	_ resource.ResourceWithImportState = &NexusEndpointResource{}
)

// NewNexusEndpointResource creates a new instance of NexusEndpointResource.
func NewNexusEndpointResource() resource.Resource {
	return &NexusEndpointResource{}
}

// NexusEndpointResource a Temporal Nexus endpoint resource implementation.
type NexusEndpointResource struct {
	client grpc.ClientConnInterface
}

// NexusEndpointResourceModel mirrors the OSS nexus.v1.EndpointSpec shape
// (name, description, target oneof) plus computed fields surfaced from
// the server-returned Endpoint (id, version, url_prefix).
type NexusEndpointResourceModel struct {
	Name           types.String `tfsdk:"name"`
	Id             types.String `tfsdk:"id"`
	Version        types.Int64  `tfsdk:"version"`
	Description    types.String `tfsdk:"description"`
	WorkerTarget   types.Object `tfsdk:"worker_target"`
	ExternalTarget types.Object `tfsdk:"external_target"`
	UrlPrefix      types.String `tfsdk:"url_prefix"`
}

// Metadata sets the metadata for the nexus_endpoint resource.
func (r *NexusEndpointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nexus_endpoint"
}

// Schema returns the schema for the Temporal Nexus endpoint resource.
func (r *NexusEndpointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Temporal Nexus Endpoint resource (self-hosted). " +
			"Maps to nexus.v1.EndpointSpec via OperatorService.{Create,Update,Delete,Get}NexusEndpoint.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Endpoint name. Globally unique within the cluster. Must match the server-enforced regex `^[a-zA-Z][a-zA-Z0-9-]*[a-zA-Z0-9]$` (letters, digits, hyphens; no underscores). " +
					"Renaming would break all callers, so this resource treats `name` as immutable; changing it forces replacement.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Server-generated unique endpoint ID.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Optimistic-concurrency-control version, incremented on each update. Used internally on Update.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Endpoint description. Encoded server-side as a `json/plain` Payload.",
				Optional:            true,
				Computed:            true,
			},
			"worker_target": schema.SingleNestedAttribute{
				MarkdownDescription: "Worker target — route operations to a handler in another namespace on this cluster. Mutually exclusive with `external_target`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"namespace": schema.StringAttribute{
						MarkdownDescription: "Namespace where the handler worker runs.",
						Required:            true,
					},
					"task_queue": schema.StringAttribute{
						MarkdownDescription: "Task queue the handler polls.",
						Required:            true,
					},
				},
			},
			"external_target": schema.SingleNestedAttribute{
				MarkdownDescription: "External target — HTTPS URL the server calls. Mutually exclusive with `worker_target`.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						MarkdownDescription: "URL to call. Must start with `http://` or `https://`; the server typically requires HTTPS in production.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(externalTargetURLRegex,
								"must start with http:// or https://"),
						},
					},
				},
			},
			"url_prefix": schema.StringAttribute{
				MarkdownDescription: "Server-rendered URL prefix for invoking operations on this endpoint. Deterministic from `id`, which is itself stable across updates, so reuse the prior state value during plan to avoid surfacing a spurious `(known after apply)` on every in-place update.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure stores the gRPC client provided by the provider.
func (r *NexusEndpointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tflog.Info(ctx, "Configured Temporal Nexus Endpoint resource", map[string]any{"success": true})
}

// Create registers a new Nexus endpoint via OperatorService.CreateNexusEndpoint.
func (r *NexusEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NexusEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, diags := buildEndpointSpec(&data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := operatorservice.NewOperatorServiceClient(r.client)
	createResp, err := client.CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{Spec: spec})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("create nexus endpoint: %s", err))
		return
	}

	updateModelFromEndpoint(&data, createResp.GetEndpoint())
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read fetches the current state of the endpoint.
func (r *NexusEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NexusEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := operatorservice.NewOperatorServiceClient(r.client)
	endpoint, err := getEndpointByIDOrName(ctx, client, state.Id.ValueString(), state.Name.ValueString())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			tflog.Warn(ctx, "Nexus endpoint not found, removing from state",
				map[string]interface{}{"name": state.Name.ValueString(), "id": state.Id.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("read nexus endpoint: %s", err))
		return
	}

	updateModelFromEndpoint(&state, endpoint)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies description and/or target. Name change forces replace via
// the schema's RequiresReplace plan modifier, so we don't handle it here.
//
// Intentionally does NOT retry on conditional-update conflicts (gRPC
// FailedPrecondition / "version mismatch"). Unlike `temporal_namespace`,
// whose `config_version` bumps from cluster-internal frontend touches even
// without user action, Nexus endpoint version increments only on real
// UpdateNexusEndpoint calls, so a conflict here is a legitimate concurrent-
// writer race that a retry could silently paper over. Surface the error.
func (r *NexusEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NexusEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state NexusEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, diags := buildEndpointSpec(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := operatorservice.NewOperatorServiceClient(r.client)
	updateResp, err := client.UpdateNexusEndpoint(ctx, &operatorservice.UpdateNexusEndpointRequest{
		Id:      state.Id.ValueString(),
		Version: state.Version.ValueInt64(),
		Spec:    spec,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("update nexus endpoint: %s", err))
		return
	}

	updateModelFromEndpoint(&plan, updateResp.GetEndpoint())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the endpoint. The Delete RPC is also OCC-versioned.
func (r *NexusEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NexusEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := operatorservice.NewOperatorServiceClient(r.client)
	_, err := client.DeleteNexusEndpoint(ctx, &operatorservice.DeleteNexusEndpointRequest{
		Id:      state.Id.ValueString(),
		Version: state.Version.ValueInt64(),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Already gone — treat as success.
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("delete nexus endpoint: %s", err))
		return
	}
}

// ImportState accepts either an endpoint ID (UUID) or a name.
//
// We seed BOTH `id` and `name` with `req.ID` because we don't know which
// form the user passed; the subsequent Read call disambiguates via
// `getEndpointByIDOrName` (UUID-shape check → GetNexusEndpoint, else
// ListNexusEndpoints by name). After Read, `id` holds the server-assigned
// UUID and `name` holds the canonical name regardless of which form was
// originally passed. The transient pre-Read state where one of the two
// fields holds the wrong value is intentional and self-correcting.
func (r *NexusEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// buildEndpointSpec converts the resource model into a gRPC EndpointSpec.
// Validates exactly-one-of(worker_target, external_target).
func buildEndpointSpec(m *NexusEndpointResourceModel) (*nexusv1.EndpointSpec, diag.Diagnostics) {
	var diags diag.Diagnostics

	hasWorker := !m.WorkerTarget.IsNull() && !m.WorkerTarget.IsUnknown()
	hasExternal := !m.ExternalTarget.IsNull() && !m.ExternalTarget.IsUnknown()

	switch {
	case !hasWorker && !hasExternal:
		diags.AddError("Invalid target", "exactly one of `worker_target` or `external_target` must be set")
		return nil, diags
	case hasWorker && hasExternal:
		diags.AddError("Invalid target", "`worker_target` and `external_target` are mutually exclusive")
		return nil, diags
	}

	spec := &nexusv1.EndpointSpec{Name: m.Name.ValueString()}

	// Allow explicitly-empty descriptions through to the server. Only skip
	// payload construction for null/unknown (the user didn't set it). The
	// round-trip path in updateModelFromEndpoint preserves the null vs ""
	// distinction by inspecting whether the server returned a payload at all.
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		payload, err := encodeDescription(m.Description.ValueString())
		if err != nil {
			diags.AddError("Encode description", fmt.Sprintf("encode nexus endpoint description: %s", err))
			return nil, diags
		}
		spec.Description = payload
	}

	if hasWorker {
		attrs := m.WorkerTarget.Attributes()
		spec.Target = &nexusv1.EndpointTarget{
			Variant: &nexusv1.EndpointTarget_Worker_{
				Worker: &nexusv1.EndpointTarget_Worker{
					Namespace: stringFromAttr(attrs["namespace"]),
					TaskQueue: stringFromAttr(attrs["task_queue"]),
				},
			},
		}
	} else {
		attrs := m.ExternalTarget.Attributes()
		spec.Target = &nexusv1.EndpointTarget{
			Variant: &nexusv1.EndpointTarget_External_{
				External: &nexusv1.EndpointTarget_External{
					Url: stringFromAttr(attrs["url"]),
				},
			},
		}
	}

	return spec, diags
}

func stringFromAttr(v attr.Value) string {
	if v == nil || v.IsNull() || v.IsUnknown() {
		return ""
	}
	if s, ok := v.(types.String); ok {
		return s.ValueString()
	}
	return ""
}

// encodeDescription wraps a UTF-8 string in a Payload with metadata
// {"encoding": "json/plain"} and a JSON-encoded string body, matching
// the Temporal CLI's encoding for Nexus endpoint descriptions.
//
// json.Marshal of a string is documented as never failing (invalid UTF-8
// is silently replaced with U+FFFD rather than erroring), so this error
// path is effectively unreachable. We still surface it as a diagnostic
// instead of panicking — providers crashing the Terraform process is
// strictly worse than a clean apply-time error message.
func encodeDescription(s string) (*commonv1.Payload, error) {
	body, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal of description string: %w", err)
	}
	return &commonv1.Payload{
		Metadata: map[string][]byte{"encoding": []byte("json/plain")},
		Data:     body,
	}, nil
}

// decodeDescription is the inverse of encodeDescription. Tolerates
// non-json/plain encodings by returning the raw bytes as a string.
func decodeDescription(p *commonv1.Payload) string {
	if p == nil {
		return ""
	}
	if string(p.GetMetadata()["encoding"]) == "json/plain" {
		var s string
		if err := json.Unmarshal(p.GetData(), &s); err == nil {
			return s
		}
	}
	return string(p.GetData())
}

// updateModelFromEndpoint populates the resource model from a server-returned Endpoint.
//
// Preserves null semantics for `description`: if the server returned no
// description payload, the model field is set to types.StringNull() rather
// than the empty-string value. This avoids a perpetual `null → ""` diff
// on every plan for endpoints that genuinely have no description.
func updateModelFromEndpoint(m *NexusEndpointResourceModel, ep *nexusv1.Endpoint) {
	if ep == nil {
		return
	}
	m.Id = types.StringValue(ep.GetId())
	m.Version = types.Int64Value(ep.GetVersion())
	m.UrlPrefix = types.StringValue(ep.GetUrlPrefix())

	spec := ep.GetSpec()
	m.Name = types.StringValue(spec.GetName())
	if desc := spec.GetDescription(); desc != nil {
		m.Description = types.StringValue(decodeDescription(desc))
	} else {
		m.Description = types.StringNull()
	}

	tgt := spec.GetTarget()
	switch {
	case tgt.GetWorker() != nil:
		obj, _ := types.ObjectValue(workerTargetAttrTypes, map[string]attr.Value{
			"namespace":  types.StringValue(tgt.GetWorker().GetNamespace()),
			"task_queue": types.StringValue(tgt.GetWorker().GetTaskQueue()),
		})
		m.WorkerTarget = obj
		m.ExternalTarget = types.ObjectNull(externalTargetAttrTypes)
	case tgt.GetExternal() != nil:
		obj, _ := types.ObjectValue(externalTargetAttrTypes, map[string]attr.Value{
			"url": types.StringValue(tgt.GetExternal().GetUrl()),
		})
		m.ExternalTarget = obj
		m.WorkerTarget = types.ObjectNull(workerTargetAttrTypes)
	default:
		m.WorkerTarget = types.ObjectNull(workerTargetAttrTypes)
		m.ExternalTarget = types.ObjectNull(externalTargetAttrTypes)
	}
}

// getEndpointByIDOrName tries Get(id) first; falls back to a Name-filtered
// List if the id is empty, looks malformed (e.g. an import string that's a
// name not a UUID), or is reported NotFound by the server.
func getEndpointByIDOrName(ctx context.Context, client operatorservice.OperatorServiceClient, id, name string) (*nexusv1.Endpoint, error) {
	// If the id field doesn't look like a UUID, skip the Get-by-id call —
	// the server validates UUID format and would reject with InvalidArgument.
	tryGetByID := id != "" && looksLikeUUID(id)

	if tryGetByID {
		getResp, err := client.GetNexusEndpoint(ctx, &operatorservice.GetNexusEndpointRequest{Id: id})
		if err == nil {
			return getResp.GetEndpoint(), nil
		}
		switch status.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			// fall through to name lookup if we have a name
		default:
			return nil, err
		}
	}

	// Use the import string itself as the name fallback if name wasn't set.
	queryName := name
	if queryName == "" {
		queryName = id
	}
	if queryName == "" {
		return nil, status.Error(codes.NotFound, "nexus endpoint not found (no id or name provided)")
	}

	listResp, err := client.ListNexusEndpoints(ctx, &operatorservice.ListNexusEndpointsRequest{Name: queryName, PageSize: 1})
	if err != nil {
		return nil, err
	}
	endpoints := listResp.GetEndpoints()
	if len(endpoints) == 0 {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("nexus endpoint %q not found", queryName))
	}
	return endpoints[0], nil
}

// looksLikeUUID does a cheap shape check (8-4-4-4-12 hex layout). Accepts
// both lowercase and uppercase hex so import-by-ID works regardless of
// the casing the user pastes (e.g. copied from a UI that uppercases UUIDs).
// Avoids pulling in a UUID library just for this.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') ||
				(c >= 'a' && c <= 'f') ||
				(c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
