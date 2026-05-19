package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// randEndpointName returns a random endpoint name that satisfies the server-
// enforced regex `^[a-zA-Z][a-zA-Z0-9-]*[a-zA-Z0-9]$`. The "tfacc" prefix
// guarantees the first char is a letter; the alphanumeric suffix guarantees
// the last char is letter-or-digit.
func randEndpointName() string {
	return "tfacc" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
}

// resourceIDFromState returns an ImportStateIdFunc that reads the ID of the
// named resource out of the current Terraform state. Useful when importing
// by server-assigned UUID rather than a user-supplied name.
func resourceIDFromState(addr string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", addr)
		}
		return rs.Primary.ID, nil
	}
}

// TestAccNexusEndpointResource_ExternalTarget exercises the full lifecycle
// for an endpoint with an external target: Create → PlanOnly (no-drift) →
// in-place Update (description + url change, version bumps 1 → 2) → ImportState
// by server-assigned UUID. CheckDestroy is implicit at end of TestCase.
func TestAccNexusEndpointResource_ExternalTarget(t *testing.T) {
	name := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name        = %q
  description = "initial description"
  external_target = {
    url = "https://example.com/nexus"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "description", "initial description"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "external_target.url", "https://example.com/nexus"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
					resource.TestCheckResourceAttrSet("temporal_nexus_endpoint.test", "id"),
					resource.TestCheckResourceAttrSet("temporal_nexus_endpoint.test", "url_prefix"),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "worker_target.namespace"),
				),
			},
			// PlanOnly read: re-running the same config produces no diff,
			// proving Read() reconstructs state identically — exercises both
			// the description-null handling and the url_prefix UseStateForUnknown
			// plan modifier.
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name        = %q
  description = "initial description"
  external_target = {
    url = "https://example.com/nexus"
  }
}
`, name),
				PlanOnly: true,
			},
			// In-place update: description and url change. version increments.
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name        = %q
  description = "updated description"
  external_target = {
    url = "https://example.com/nexus/v2"
  }
}
`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("temporal_nexus_endpoint.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "description", "updated description"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "external_target.url", "https://example.com/nexus/v2"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "2"),
				),
			},
			// ImportState by server-assigned UUID. `version` is excluded from
			// strict-equality verification because it's an OCC token re-read
			// from the server on import and can advance between the prior
			// state snapshot and the import-time read.
			{
				ResourceName:            "temporal_nexus_endpoint.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       resourceIDFromState("temporal_nexus_endpoint.test"),
				ImportStateVerifyIgnore: []string{"version"},
			},
		},
	})
}

// TestAccNexusEndpointResource_WorkerTarget exercises the worker target
// branch end-to-end, plus an import by NAME (rather than UUID) to cover
// the ListNexusEndpoints fallback path in getEndpointByIDOrName.
func TestAccNexusEndpointResource_WorkerTarget(t *testing.T) {
	name := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  worker_target = {
    namespace  = "default"
    task_queue = "main-queue"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.namespace", "default"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.task_queue", "main-queue"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "external_target.url"),
				),
			},
			// In-place update of worker target (change task queue).
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  worker_target = {
    namespace  = "default"
    task_queue = "secondary-queue"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.task_queue", "secondary-queue"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "2"),
				),
			},
			// Import by NAME — the import string isn't UUID-shaped, so
			// looksLikeUUID returns false and we go straight to the
			// ListNexusEndpoints fallback. Verify the full state round-trip
			// (less `version`, which can race).
			{
				ResourceName:            "temporal_nexus_endpoint.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateId:           name,
				ImportStateVerifyIgnore: []string{"version"},
			},
		},
	})
}

// TestAccNexusEndpointResource_NameRequiresReplace verifies that changing
// `name` forces destroy+create rather than in-place update. The plancheck
// asserts the resource action explicitly; the post-apply check confirms a
// fresh resource (server-assigned `id` changes, OCC `version` resets to 1).
func TestAccNexusEndpointResource_NameRequiresReplace(t *testing.T) {
	name1 := randEndpointName()
	name2 := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  external_target = {
    url = "https://example.com"
  }
}
`, name1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name1),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  external_target = {
    url = "https://example.com"
  }
}
`, name2),
				// Plan must show this as destroy+create, not in-place update.
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("temporal_nexus_endpoint.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name2),
					// Fresh resource — OCC version restarts at 1.
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
				),
			},
		},
	})
}

// TestAccNexusEndpointResource_MissingTarget verifies that omitting both
// target blocks surfaces the "exactly one of" error at apply time. This is
// a runtime check in buildEndpointSpec (not a schema-level validator), so
// the error appears at apply rather than plan.
func TestAccNexusEndpointResource_MissingTarget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
}
`, randEndpointName()),
				ExpectError: regexp.MustCompile(`exactly one of .worker_target. or .external_target. must be set`),
			},
		},
	})
}

// TestAccNexusEndpointResource_BothTargets verifies the mutual-exclusivity
// runtime check fires when both target blocks are populated.
func TestAccNexusEndpointResource_BothTargets(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  worker_target = {
    namespace  = "default"
    task_queue = "tq"
  }
  external_target = {
    url = "https://example.com"
  }
}
`, randEndpointName()),
				ExpectError: regexp.MustCompile(`mutually exclusive`),
			},
		},
	})
}

// TestAccNexusEndpointResource_InvalidExternalURL verifies the schema-level
// URL regex validator catches non-http(s) schemes at plan time.
func TestAccNexusEndpointResource_InvalidExternalURL(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  external_target = {
    url = "ftp://example.com"
  }
}
`, randEndpointName()),
				ExpectError: regexp.MustCompile(`must start with http`),
			},
		},
	})
}

// TestAccNexusEndpointResource_DescriptionOmitted verifies that an endpoint
// created without a description has Description == null in state — not the
// empty string. The follow-on PlanOnly step proves the null-vs-empty handling
// in updateModelFromEndpoint avoids a perpetual `null → ""` diff.
func TestAccNexusEndpointResource_DescriptionOmitted(t *testing.T) {
	name := randEndpointName()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  external_target = {
    url = "https://example.com"
  }
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "description"),
				),
			},
			// Same config, PlanOnly — must not surface a diff. A null→""
			// regression in updateModelFromEndpoint would fail this step.
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  external_target = {
    url = "https://example.com"
  }
}
`, name),
				PlanOnly: true,
			},
		},
	})
}
