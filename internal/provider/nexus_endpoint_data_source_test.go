package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNexusEndpointDataSource_WorkerTarget(t *testing.T) {
	name := testAccNexusEndpointName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNexusEndpointWorkerConfig(name, `description = "read me"`, "queue") + `
data "temporal_nexus_endpoint" "test" {
  name = temporal_nexus_endpoint.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "name", name),
					resource.TestCheckResourceAttrPair("data.temporal_nexus_endpoint.test", "id", "temporal_nexus_endpoint.test", "id"),
					resource.TestCheckResourceAttrPair("data.temporal_nexus_endpoint.test", "url_prefix", "temporal_nexus_endpoint.test", "url_prefix"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "description", "read me"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "worker_target.namespace", "default"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "worker_target.task_queue", "queue"),
					resource.TestCheckNoResourceAttr("data.temporal_nexus_endpoint.test", "external_target.url"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "version", "1"),
				),
			},
		},
	})
}

func TestAccNexusEndpointDataSource_ExternalTarget(t *testing.T) {
	name := testAccNexusEndpointName()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNexusEndpointExternalConfig(name, "", "https://example.com/nexus") + `
data "temporal_nexus_endpoint" "test" {
  name = temporal_nexus_endpoint.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.temporal_nexus_endpoint.test", "id", "temporal_nexus_endpoint.test", "id"),
					resource.TestCheckNoResourceAttr("data.temporal_nexus_endpoint.test", "description"),
					resource.TestCheckResourceAttr("data.temporal_nexus_endpoint.test", "external_target.url", "https://example.com/nexus"),
					resource.TestCheckNoResourceAttr("data.temporal_nexus_endpoint.test", "worker_target.namespace"),
				),
			},
		},
	})
}

func TestAccNexusEndpointDataSource_NotFound(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "temporal_nexus_endpoint" "missing" {
  name = %q
}
`, testAccNexusEndpointName()),
				ExpectError: regexp.MustCompile("Could not read Nexus endpoint"),
			},
		},
	})
}
