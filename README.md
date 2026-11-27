# Terraform Provider Temporal

This provider aims to manage [Temporal](https://temporal.io/) resources.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.26

## Building the provider

1. Clone the repository.
2. Enter the repository directory.
3. Build the provider using the Go `install` command:

   ```shell
   go install
   ```

## Using the provider

Usage examples can be found in the Terraform provider
[documentation](https://registry.terraform.io/providers/platacard/temporal/latest/docs).

> [!NOTE]
> The existing `retention` attribute remains an integer number of days for
> backward compatibility and is deprecated. Use `retention_period` for duration
> precision:
>
> ```hcl
> resource "temporal_namespace" "example" {
>   name             = "example"
>   owner_email      = "team@example.org"
>   retention_period = "90m30s"
> }
> ```
>
> - Existing configuration, state, and numeric data source outputs for
>   `retention` continue to work.
> - Configure either `retention` or `retention_period`; setting both is
>   rejected.
> - Temporal enforces a minimum of one hour for local namespaces and one day
>   for global namespaces.
> - Duration strings use Go syntax, so use `72h` instead of `3d`.
> - When neither attribute is configured, the plan explicitly defaults to three
>   days (`retention = 3`, `retention_period = "72h0m0s"`). Removing either
>   configured attribute therefore plans a return to three days.
> - The deprecated `retention` field is intended for removal in a future major
>   release.

## Developing the provider

If you wish to work on the provider, you'll first need
[Go](http://www.golang.org) installed on your machine (see
[Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put
the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of acceptance tests, run `make testacc`:

```shell
make testacc
```
