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

## Configuration

```hcl
provider "cfdev" {
  # Prefer CLOUDFLARE_API_TOKEN in the environment instead of committing a token.
  cloudflare_api_token = var.cloudflare_api_token

  # Optional; defaults to https://api.cloudflare.com/client/v4.
  # cloudflare_api_endpoint = "https://api.cloudflare.com/client/v4"
}
```

`cloudflare_api_token` is sensitive and may instead be supplied through the
`CLOUDFLARE_API_TOKEN` environment variable. `cloudflare_api_endpoint` supports
compatible alternate Cloudflare API endpoints.

The provider currently has no managed resources; D1 migration support will be added in
subsequent releases. Networked acceptance tests and Terraform configuration validation
remain intentionally inapplicable until a resource and example configuration exist.

## License

This project is licensed under the [MIT License](LICENSE).
