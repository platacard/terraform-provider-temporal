package provider

import (
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	commonv1 "go.temporal.io/api/common/v1"
	nexusv1 "go.temporal.io/api/nexus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCanonicalUUID(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"0ac9f14a-3fb8-4526-90be-854c798f9582": {"0ac9f14a-3fb8-4526-90be-854c798f9582", true},
		"0AC9F14A-3FB8-4526-90BE-854C798F9582": {"0ac9f14a-3fb8-4526-90be-854c798f9582", true},
		"my-endpoint":                          {"", false},
		"0ac9f14a3fb8452690be854c798f9582":     {"", false},
		"":                                     {"", false},
	}
	for input, want := range cases {
		got, ok := canonicalUUID(input)
		if got != want.want || ok != want.ok {
			t.Errorf("canonicalUUID(%q): got (%q, %t), want (%q, %t)", input, got, ok, want.want, want.ok)
		}
	}
}

func TestIsNexusEndpointNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"missing endpoint", status.Error(codes.NotFound, "Nexus endpoint with ID `x` not found"), true},
		{"Nexus disabled on the server", status.Error(codes.NotFound, nexusAPIsDisabledMessage), false},
		{"other status", status.Error(codes.Internal, "boom"), false},
		{"plain error", errors.New("boom"), false},
		{"no error", nil, false},
	}
	for _, c := range cases {
		if got := isNexusEndpointNotFound(c.err); got != c.want {
			t.Errorf("%s: got %t, want %t", c.name, got, c.want)
		}
	}
}

func TestNexusEndpointDescriptionPayload(t *testing.T) {
	if payload := nexusEndpointDescriptionPayload(types.StringNull()); payload != nil {
		t.Errorf("null description: expected no payload, got %v", payload)
	}

	cases := map[string]string{
		"":             `""`,
		"**markdown**": `"**markdown**"`,
	}
	for input, want := range cases {
		payload := nexusEndpointDescriptionPayload(types.StringValue(input))
		if payload == nil {
			t.Fatalf("%q: expected a payload", input)
		}
		if got := string(payload.GetData()); got != want {
			t.Errorf("%q: got data %s, want %s", input, got, want)
		}
		if got := string(payload.GetMetadata()["encoding"]); got != "json/plain" {
			t.Errorf("%q: got encoding %q, want json/plain", input, got)
		}

		decoded, diags := nexusEndpointDescription(payload)
		if len(diags) != 0 {
			t.Errorf("%q: unexpected diagnostics: %v", input, diags)
		}
		if decoded.IsNull() || decoded.ValueString() != input {
			t.Errorf("%q: decoded back as %s", input, decoded)
		}
	}
}

func TestNexusEndpointDescription(t *testing.T) {
	description, diags := nexusEndpointDescription(nil)
	if !description.IsNull() || len(diags) != 0 {
		t.Errorf("missing payload: got %s with diagnostics %v, want null", description, diags)
	}

	description, diags = nexusEndpointDescription(&commonv1.Payload{Data: []byte("plain text")})
	if description.ValueString() != "plain text" || len(diags) != 0 {
		t.Errorf("text payload: got %s with diagnostics %v", description, diags)
	}

	description, diags = nexusEndpointDescription(&commonv1.Payload{Data: []byte{0xff, 0xfe}})
	if description.ValueString() != "//4=" {
		t.Errorf("binary payload: got %s, want base64", description)
	}
	if diags.WarningsCount() != 1 || diags.HasError() {
		t.Errorf("binary payload: expected one warning, got %v", diags)
	}
}

func TestNexusEndpointModel_EndpointSpec(t *testing.T) {
	worker := NexusEndpointModel{
		Name:        types.StringValue("worker-endpoint"),
		Description: types.StringValue("to workers"),
		WorkerTarget: &NexusEndpointWorkerTargetModel{
			Namespace: types.StringValue("default"),
			TaskQueue: types.StringValue("queue"),
		},
	}
	spec := worker.endpointSpec()
	if spec.GetName() != "worker-endpoint" || spec.GetDescription() == nil {
		t.Errorf("unexpected worker spec: %v", spec)
	}
	if target := spec.GetTarget().GetWorker(); target.GetNamespace() != "default" || target.GetTaskQueue() != "queue" {
		t.Errorf("unexpected worker target: %v", spec.GetTarget())
	}
	if spec.GetTarget().GetExternal() != nil {
		t.Error("worker spec must not have an external target")
	}

	external := NexusEndpointModel{
		Name:           types.StringValue("external-endpoint"),
		Description:    types.StringNull(),
		ExternalTarget: &NexusEndpointExternalTargetModel{Url: types.StringValue("https://example.com/nexus")},
	}
	spec = external.endpointSpec()
	if spec.GetDescription() != nil {
		t.Errorf("expected no description, got %v", spec.GetDescription())
	}
	if spec.GetTarget().GetExternal().GetUrl() != "https://example.com/nexus" || spec.GetTarget().GetWorker() != nil {
		t.Errorf("unexpected external target: %v", spec.GetTarget())
	}
}

func TestNexusEndpointModel_SetFromEndpoint(t *testing.T) {
	model := NexusEndpointModel{
		Description:  types.StringValue("stale"),
		WorkerTarget: &NexusEndpointWorkerTargetModel{Namespace: types.StringValue("default"), TaskQueue: types.StringValue("queue")},
	}

	diags := model.setFromEndpoint(&nexusv1.Endpoint{
		Id:        "0ac9f14a-3fb8-4526-90be-854c798f9582",
		Version:   3,
		UrlPrefix: "/nexus/endpoints/0ac9f14a-3fb8-4526-90be-854c798f9582/services",
		Spec: &nexusv1.EndpointSpec{
			Name: "external-endpoint",
			Target: &nexusv1.EndpointTarget{Variant: &nexusv1.EndpointTarget_External_{
				External: &nexusv1.EndpointTarget_External{Url: "https://example.com/nexus"},
			}},
		},
	})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if model.Id.ValueString() != "0ac9f14a-3fb8-4526-90be-854c798f9582" || model.Name.ValueString() != "external-endpoint" || model.Version.ValueInt64() != 3 {
		t.Errorf("unexpected model: %+v", model)
	}
	if !model.Description.IsNull() {
		t.Errorf("expected a null description, got %s", model.Description)
	}
	if model.WorkerTarget != nil {
		t.Error("expected the previous worker target to be cleared")
	}
	if model.ExternalTarget == nil || model.ExternalTarget.Url.ValueString() != "https://example.com/nexus" {
		t.Errorf("unexpected external target: %+v", model.ExternalTarget)
	}

	if diags := model.setFromEndpoint(nil); len(diags) != 0 {
		t.Errorf("nil endpoint: unexpected diagnostics: %v", diags)
	}
	if model.ExternalTarget != nil || model.WorkerTarget != nil {
		t.Error("nil endpoint: expected no targets")
	}
}
