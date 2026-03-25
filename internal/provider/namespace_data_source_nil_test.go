package provider

import (
	"context"
	"testing"

	namespacev1 "go.temporal.io/api/namespace/v1"
	replicationv1 "go.temporal.io/api/replication/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
)

// TestNamespaceDataSourceModel_NilConfig verifies that namespaceToNamespaceDataSourceModel
// does not panic when ns.Config is nil. Before the fix the function accessed
// ns.Config.WorkflowExecutionRetentionTtl directly, which panicked
func TestNamespaceDataSourceModel_NilConfig(t *testing.T) {
	ctx := context.Background()
	ns := &workflowservice.DescribeNamespaceResponse{
		NamespaceInfo:     &namespacev1.NamespaceInfo{Name: "test-ns"},
		Config:            nil,
		ReplicationConfig: &replicationv1.NamespaceReplicationConfig{},
	}

	model, diags := namespaceToNamespaceDataSourceModel(ctx, ns)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if model == nil {
		t.Fatal("expected non-nil model")
	}
	if model.Retention.ValueInt64() != 0 {
		t.Errorf("expected Retention 0 for nil Config, got %d", model.Retention.ValueInt64())
	}
}

// TestNamespaceDataSourceModel_EmptyClusters verifies that the Clusters field is
// a typed empty list (not null) when no clusters are configured. A null or
// zero-value list causes Terraform Framework to detect a perpetual diff
func TestNamespaceDataSourceModel_EmptyClusters(t *testing.T) {
	ctx := context.Background()
	ns := &workflowservice.DescribeNamespaceResponse{
		NamespaceInfo:     &namespacev1.NamespaceInfo{Name: "test-ns"},
		Config:            &namespacev1.NamespaceConfig{},
		ReplicationConfig: &replicationv1.NamespaceReplicationConfig{},
	}

	model, diags := namespaceToNamespaceDataSourceModel(ctx, ns)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if model.Clusters.IsNull() {
		t.Error("Clusters must be a typed empty list, not null — would cause perpetual state drift")
	}
	if len(model.Clusters.Elements()) != 0 {
		t.Errorf("expected 0 cluster elements, got %d", len(model.Clusters.Elements()))
	}
}
