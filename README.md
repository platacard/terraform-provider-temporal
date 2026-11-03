# Terraform Provider Temporal

This provider aims to manage [Temporal](https://temporal.io/) resources

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.19

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Using the provider

Usage examples can be found in the Terraform provider [documentation](https://registry.terraform.io/providers/platacard/temporal/latest/docs)

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

### Namespace retention durations

Namespace retention accepts Go duration strings, including subday values:

```hcl
resource "temporal_namespace" "example" {
  name        = "example"
  owner_email = "team@example.org"
  retention   = "90m30s"
}
```

Temporal enforces a minimum of one hour for local namespaces and one day for
global namespaces. Use hours for days (for example, `"72h"`); the duration syntax
does not include a `d` unit.

#### Migrating integer-day retention

Existing resource state is automatically upgraded from integer days to strings.
Existing configurations such as `retention = 3` remain supported and still mean
three days. The default remains three days (`"72h"`). Equivalent configured
spellings are preserved during refresh.

The resource and data source now expose retention as a string. Update consumers
that previously performed numeric operations on this attribute. Newly imported
resources and data source results use Go's duration formatting (for example,
`"1h0m1s"`). Update legacy configurations to duration strings when convenient;
no namespace recreation is required.
