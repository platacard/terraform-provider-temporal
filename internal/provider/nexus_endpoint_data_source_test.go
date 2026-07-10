package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccNexusEndpointDataSource_WorkerTarget creates an endpoint via the
// resource then reads it back via the data source, asserting every computed
// field is populated and the worker-target shape is correct.
func TestAccNexusEndpointDataSource_WorkerTarget(t *testing.T) {
	name := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "src" {
  name        = %q
  description = "data source test"
  worker_target = {
    namespace  = "default"
    task_queue = "ds-test-queue"
  }
}

data "temporal_nexus_endpoint" "by_name" {
  name = temporal_nexus_endpoint.src.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "name", name),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "description", "data source test"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "worker_target.namespace", "default"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "worker_target.task_queue", "ds-test-queue"),
					resource.TestCheckResourceAttrSet("data.temporal_nexus_endpoint.by_name", "id"),
					resource.TestCheckResourceAttrSet("data.temporal_nexus_endpoint.by_name", "url_prefix"),
					resource.TestCheckResourceAttrSet("data.temporal_nexus_endpoint.by_name", "version"),
					// The data source and the resource should agree on id/url_prefix.
					resource.TestCheckResourceAttrPair(
						"data.temporal_nexus_endpoint.by_name", "id",
						"temporal_nexus_endpoint.src", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.temporal_nexus_endpoint.by_name", "url_prefix",
						"temporal_nexus_endpoint.src", "url_prefix",
					),
				),
			},
		},
	})
}

// TestAccNexusEndpointDataSource_ExternalTarget mirrors the worker case but
// for an external-target endpoint, ensuring the external branch of
// updateModelFromEndpoint surfaces correctly through the data source.
func TestAccNexusEndpointDataSource_ExternalTarget(t *testing.T) {
	name := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "src" {
  name = %q
  external_target = {
    url = "https://example.com/nexus/ds"
  }
}

data "temporal_nexus_endpoint" "by_name" {
  name = temporal_nexus_endpoint.src.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "name", name),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.by_name", "external_target.url", "https://example.com/nexus/ds"),
					resource.TestCheckNoResourceAttr("data.temporal_nexus_endpoint.by_name", "worker_target.namespace"),
				),
			},
		},
	})
}

// TestAccNexusEndpointDataSource_NotFound verifies the data source surfaces
// a clear error when the endpoint name doesn't exist.
func TestAccNexusEndpointDataSource_NotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
data "temporal_nexus_endpoint" "missing" {
  name = "definitely-does-not-exist-xyz123"
}
`,
				ExpectError: regexp.MustCompile(`not found`),
			},
		},
	})
}
