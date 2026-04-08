package provider

import (
	"context"
	"testing"

	namespacev1 "go.temporal.io/api/namespace/v1"
	replicationv1 "go.temporal.io/api/replication/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
)

// TestUpdateModelFromSpec_EmptyClusters verifies that updateModelFromSpec sets
// Clusters to a typed empty list (not a zero-value types.List{}) when the
// namespace has no replication clusters. A zero-value list has no element type
// and causes Terraform Framework to detect a perpetual diff.
func TestUpdateModelFromSpec_EmptyClusters(t *testing.T) {
	ctx := context.Background()
	data := &NamespaceResourceModel{}
	ns := &workflowservice.DescribeNamespaceResponse{
		NamespaceInfo:     &namespacev1.NamespaceInfo{},
		Config:            &namespacev1.NamespaceConfig{},
		ReplicationConfig: &replicationv1.NamespaceReplicationConfig{},
	}

	diags := updateModelFromSpec(ctx, data, ns)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if data.Clusters.IsNull() {
		t.Error("Clusters must not be null — would cause perpetual state drift")
	}
	if data.Clusters.IsUnknown() {
		t.Error("Clusters must not be unknown")
	}
	if len(data.Clusters.Elements()) != 0 {
		t.Errorf("expected 0 cluster elements, got %d", len(data.Clusters.Elements()))
	}
}

// TestUpdateModelFromSpec_WithClusters verifies that updateModelFromSpec correctly
// populates Clusters when the namespace has replication clusters.
func TestUpdateModelFromSpec_WithClusters(t *testing.T) {
	ctx := context.Background()
	data := &NamespaceResourceModel{}
	ns := &workflowservice.DescribeNamespaceResponse{
		NamespaceInfo: &namespacev1.NamespaceInfo{},
		Config:        &namespacev1.NamespaceConfig{},
		ReplicationConfig: &replicationv1.NamespaceReplicationConfig{
			ActiveClusterName: "active",
			Clusters: []*replicationv1.ClusterReplicationConfig{
				{ClusterName: "active"},
				{ClusterName: "standby"},
			},
		},
	}

	diags := updateModelFromSpec(ctx, data, ns)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(data.Clusters.Elements()) != 2 {
		t.Errorf("expected 2 cluster elements, got %d", len(data.Clusters.Elements()))
	}
}
