package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNamespaceDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + `
data "temporal_namespace" "default" {
	name = "default"
}
`,

				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_namespace.default", "name", "default"),
					resource.TestCheckResourceAttr("data.temporal_namespace.default", "description", ""),
				),
			},
		},
	})
}
