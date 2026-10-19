// Copyright 2026 Adrian Hall
// SPDX-License-Identifier: MPL-2.0

// Package provider implements the cfdev Terraform provider.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &cfdevProvider{}

// New returns the provider factory consumed by the Terraform provider server.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cfdevProvider{version: version}
	}
}

type cfdevProvider struct {
	version string
}

func (p *cfdevProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cfdev"
	resp.Version = p.version
}

func (p *cfdevProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The cfdev provider manages Cloudflare developer resources.",
	}
}

func (p *cfdevProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *cfdevProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *cfdevProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
