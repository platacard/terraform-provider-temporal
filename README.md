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

The existing `retention` attribute remains an integer number of days for
backward compatibility and is deprecated. Use `retention_period` for duration
precision:

```hcl
resource "temporal_namespace" "example" {
  name             = "example"
  owner_email      = "team@example.org"
  retention        = 3
  retention_period = "90m30s"
}
```

Existing configuration, state, and numeric data source outputs for `retention`
continue to work. When `retention_period` is set, it determines the Temporal
retention. Temporal enforces a minimum of one hour for local namespaces and one
day for global namespaces. Duration strings use Go syntax, so use `72h` instead
of `3d`. The default remains three days. The deprecated `retention` field is
intended for removal in a future major release.
