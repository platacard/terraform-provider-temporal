package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	commonv1 "go.temporal.io/api/common/v1"
	nexusv1 "go.temporal.io/api/nexus/v1"
)

// --- description encode/decode ---

func TestEncodeDecodeDescription_Roundtrip(t *testing.T) {
	cases := []string{
		"hello world",
		"",
		"unicode: ☃ 漢字 emoji 🎉",
		"with \"quotes\" and \\backslashes\\",
		"multi\nline\nstring",
	}
	for _, want := range cases {
		t.Run(want, func(t *testing.T) {
			got := decodeDescription(encodeDescription(want))
			if got != want {
				t.Errorf("roundtrip: got %q, want %q", got, want)
			}
		})
	}
}

func TestEncodeDescription_MetadataEncoding(t *testing.T) {
	p := encodeDescription("hello")
	if string(p.GetMetadata()["encoding"]) != "json/plain" {
		t.Errorf("encoding metadata: got %q, want %q",
			p.GetMetadata()["encoding"], "json/plain")
	}
	// Data should be JSON-encoded (quoted), not raw.
	if string(p.GetData()) != `"hello"` {
		t.Errorf("data: got %q, want %q", p.GetData(), `"hello"`)
	}
}

func TestDecodeDescription_NilPayload(t *testing.T) {
	if got := decodeDescription(nil); got != "" {
		t.Errorf("nil payload: got %q, want \"\"", got)
	}
}

func TestDecodeDescription_NonJSONPlainEncoding(t *testing.T) {
	p := &commonv1.Payload{
		Metadata: map[string][]byte{"encoding": []byte("binary/null")},
		Data:     []byte("raw-bytes"),
	}
	if got := decodeDescription(p); got != "raw-bytes" {
		t.Errorf("non-json/plain: got %q, want %q", got, "raw-bytes")
	}
}

func TestDecodeDescription_MalformedJSONPlain(t *testing.T) {
	// Metadata claims json/plain but data is not valid JSON — should fall
	// through to raw-bytes path rather than panicking or returning "".
	p := &commonv1.Payload{
		Metadata: map[string][]byte{"encoding": []byte("json/plain")},
		Data:     []byte("not-quoted"),
	}
	if got := decodeDescription(p); got != "not-quoted" {
		t.Errorf("malformed json/plain: got %q, want %q", got, "not-quoted")
	}
}

// --- looksLikeUUID ---

func TestLooksLikeUUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"3b9aa8e2-2c6d-4a48-9c1e-a1b2c3d4e5f6", true}, // canonical lowercase
		{"3B9AA8E2-2C6D-4A48-9C1E-A1B2C3D4E5F6", false}, // uppercase rejected
		{"", false},
		{"not-a-uuid", false},
		{"3b9aa8e22c6d4a489c1ea1b2c3d4e5f6", false}, // missing hyphens
		{"3b9aa8e2-2c6d-4a48-9c1e-a1b2c3d4e5f", false}, // 35 chars
		{"3b9aa8e2-2c6d-4a48-9c1e-a1b2c3d4e5f6a", false}, // 37 chars
		{"3b9aa8e2x2c6d-4a48-9c1e-a1b2c3d4e5f6", false}, // wrong hyphen position
		{"3b9aa8e2-2c6d-4a48-9c1e-a1b2c3d4e5fg", false}, // non-hex char
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := looksLikeUUID(tc.in); got != tc.want {
				t.Errorf("looksLikeUUID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- buildEndpointSpec ---

func workerTargetObject(t *testing.T, namespace, taskQueue string) types.Object {
	t.Helper()
	obj, diags := types.ObjectValue(workerTargetAttrTypes, map[string]attr.Value{
		"namespace":  types.StringValue(namespace),
		"task_queue": types.StringValue(taskQueue),
	})
	if diags.HasError() {
		t.Fatalf("workerTargetObject: %v", diags)
	}
	return obj
}

func externalTargetObject(t *testing.T, url string) types.Object {
	t.Helper()
	obj, diags := types.ObjectValue(externalTargetAttrTypes, map[string]attr.Value{
		"url": types.StringValue(url),
	})
	if diags.HasError() {
		t.Fatalf("externalTargetObject: %v", diags)
	}
	return obj
}

func TestBuildEndpointSpec_NoTarget(t *testing.T) {
	m := &NexusEndpointResourceModel{
		Name:           types.StringValue("ep"),
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
	}
	_, diags := buildEndpointSpec(m)
	if !diags.HasError() {
		t.Fatal("expected error for missing target, got none")
	}
}

func TestBuildEndpointSpec_BothTargets(t *testing.T) {
	m := &NexusEndpointResourceModel{
		Name:           types.StringValue("ep"),
		WorkerTarget:   workerTargetObject(t, "default", "tq"),
		ExternalTarget: externalTargetObject(t, "https://example.com"),
	}
	_, diags := buildEndpointSpec(m)
	if !diags.HasError() {
		t.Fatal("expected mutual-exclusivity error, got none")
	}
}

func TestBuildEndpointSpec_WorkerTarget(t *testing.T) {
	m := &NexusEndpointResourceModel{
		Name:           types.StringValue("worker-ep"),
		Description:    types.StringValue("a description"),
		WorkerTarget:   workerTargetObject(t, "default", "main-queue"),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
	}
	spec, diags := buildEndpointSpec(m)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if spec.GetName() != "worker-ep" {
		t.Errorf("name: got %q, want %q", spec.GetName(), "worker-ep")
	}
	worker := spec.GetTarget().GetWorker()
	if worker == nil {
		t.Fatal("expected worker target, got nil")
	}
	if worker.GetNamespace() != "default" {
		t.Errorf("namespace: got %q, want %q", worker.GetNamespace(), "default")
	}
	if worker.GetTaskQueue() != "main-queue" {
		t.Errorf("task_queue: got %q, want %q", worker.GetTaskQueue(), "main-queue")
	}
	if spec.GetTarget().GetExternal() != nil {
		t.Error("expected external target to be nil")
	}
	if spec.GetDescription() == nil {
		t.Error("expected description payload, got nil")
	}
}

func TestBuildEndpointSpec_ExternalTarget(t *testing.T) {
	m := &NexusEndpointResourceModel{
		Name:           types.StringValue("ext-ep"),
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: externalTargetObject(t, "https://example.com/nexus"),
	}
	spec, diags := buildEndpointSpec(m)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	ext := spec.GetTarget().GetExternal()
	if ext == nil {
		t.Fatal("expected external target, got nil")
	}
	if ext.GetUrl() != "https://example.com/nexus" {
		t.Errorf("url: got %q, want %q", ext.GetUrl(), "https://example.com/nexus")
	}
}

func TestBuildEndpointSpec_DescriptionEmptyOrNull(t *testing.T) {
	cases := []struct {
		name string
		desc types.String
	}{
		{"null", types.StringNull()},
		{"empty", types.StringValue("")},
		{"unknown", types.StringUnknown()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &NexusEndpointResourceModel{
				Name:           types.StringValue("ep"),
				Description:    tc.desc,
				ExternalTarget: externalTargetObject(t, "https://example.com"),
				WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
			}
			spec, diags := buildEndpointSpec(m)
			if diags.HasError() {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if spec.GetDescription() != nil {
				t.Errorf("expected nil description payload for %s, got %+v",
					tc.name, spec.GetDescription())
			}
		})
	}
}

// --- updateModelFromEndpoint ---

func TestUpdateModelFromEndpoint_NilEndpointNoPanic(t *testing.T) {
	m := &NexusEndpointResourceModel{
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
	}
	updateModelFromEndpoint(m, nil)
	// No panic, no assertion needed — just that the call doesn't crash.
}

func TestUpdateModelFromEndpoint_WorkerTarget(t *testing.T) {
	m := &NexusEndpointResourceModel{
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
	}
	ep := &nexusv1.Endpoint{
		Id:        "abc-id",
		Version:   3,
		UrlPrefix: "/api/v1/namespaces/default/nexus/endpoints/abc-id",
		Spec: &nexusv1.EndpointSpec{
			Name:        "wep",
			Description: encodeDescription("hello"),
			Target: &nexusv1.EndpointTarget{
				Variant: &nexusv1.EndpointTarget_Worker_{
					Worker: &nexusv1.EndpointTarget_Worker{
						Namespace: "default",
						TaskQueue: "main",
					},
				},
			},
		},
	}
	updateModelFromEndpoint(m, ep)

	if m.Id.ValueString() != "abc-id" {
		t.Errorf("id: got %q, want %q", m.Id.ValueString(), "abc-id")
	}
	if m.Version.ValueInt64() != 3 {
		t.Errorf("version: got %d, want 3", m.Version.ValueInt64())
	}
	if m.Name.ValueString() != "wep" {
		t.Errorf("name: got %q, want %q", m.Name.ValueString(), "wep")
	}
	if m.Description.ValueString() != "hello" {
		t.Errorf("description: got %q, want %q", m.Description.ValueString(), "hello")
	}
	if m.WorkerTarget.IsNull() {
		t.Error("expected WorkerTarget populated, got null")
	}
	if !m.ExternalTarget.IsNull() {
		t.Error("expected ExternalTarget null when worker target is set")
	}
	if got := m.WorkerTarget.Attributes()["namespace"].(types.String).ValueString(); got != "default" {
		t.Errorf("worker namespace: got %q, want %q", got, "default")
	}
	if got := m.WorkerTarget.Attributes()["task_queue"].(types.String).ValueString(); got != "main" {
		t.Errorf("worker task_queue: got %q, want %q", got, "main")
	}
}

func TestUpdateModelFromEndpoint_ExternalTarget(t *testing.T) {
	m := &NexusEndpointResourceModel{
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
	}
	ep := &nexusv1.Endpoint{
		Id:      "ext-id",
		Version: 1,
		Spec: &nexusv1.EndpointSpec{
			Name: "extep",
			Target: &nexusv1.EndpointTarget{
				Variant: &nexusv1.EndpointTarget_External_{
					External: &nexusv1.EndpointTarget_External{
						Url: "https://example.com/nexus",
					},
				},
			},
		},
	}
	updateModelFromEndpoint(m, ep)

	if m.ExternalTarget.IsNull() {
		t.Error("expected ExternalTarget populated, got null")
	}
	if !m.WorkerTarget.IsNull() {
		t.Error("expected WorkerTarget null when external target is set")
	}
	if got := m.ExternalTarget.Attributes()["url"].(types.String).ValueString(); got != "https://example.com/nexus" {
		t.Errorf("external url: got %q, want %q", got, "https://example.com/nexus")
	}
}

// TestUpdateModelFromEndpoint_NoDescriptionStaysNull verifies that a missing
// description payload sets Description to StringNull, not StringValue("").
// Returning "" would produce a perpetual `null → ""` diff on every plan.
func TestUpdateModelFromEndpoint_NoDescriptionStaysNull(t *testing.T) {
	m := &NexusEndpointResourceModel{
		WorkerTarget:   types.ObjectNull(workerTargetAttrTypes),
		ExternalTarget: types.ObjectNull(externalTargetAttrTypes),
		Description:    types.StringValue("stale-value"), // simulate prior state
	}
	ep := &nexusv1.Endpoint{
		Spec: &nexusv1.EndpointSpec{
			Name: "ep",
			// no Description
			Target: &nexusv1.EndpointTarget{
				Variant: &nexusv1.EndpointTarget_External_{
					External: &nexusv1.EndpointTarget_External{Url: "https://x"},
				},
			},
		},
	}
	updateModelFromEndpoint(m, ep)
	if !m.Description.IsNull() {
		t.Errorf("expected Description to be null after read of endpoint with no description, got %q", m.Description.ValueString())
	}
}

// --- stringFromAttr ---

func TestStringFromAttr(t *testing.T) {
	cases := []struct {
		name string
		in   attr.Value
		want string
	}{
		{"nil", nil, ""},
		{"null string", types.StringNull(), ""},
		{"unknown string", types.StringUnknown(), ""},
		{"value", types.StringValue("hello"), "hello"},
		{"non-string", types.Int64Value(42), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringFromAttr(tc.in); got != tc.want {
				t.Errorf("stringFromAttr(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
