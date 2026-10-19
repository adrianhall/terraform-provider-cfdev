# Terraform Provider cfdev

The `cfdev` provider manages Cloudflare developer resources.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) 1.10 or later.
- Go 1.27 or later to build from source.

## Installation

```hcl
terraform {
  required_providers {
    cfdev = {
      source = "adrianhall/cfdev"
    }
  }
}
```

## Development

```sh
gofmt -w $(git ls-files '*.go')
go vet ./...
go test -race ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

The initial provider has no configurable arguments or managed resources. D1 migration
support will be added in subsequent releases. This foundation's provider package is
covered by unit tests; networked acceptance tests and Terraform configuration validation
are intentionally not applicable until a resource and an example configuration exist.

## License

This project is licensed under the [Mozilla Public License 2.0](LICENSE).
