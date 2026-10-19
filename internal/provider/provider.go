// Copyright 2026 Adrian Hall
// SPDX-License-Identifier: MIT

// Package provider implements the cfdev Terraform provider.
package provider

import (
	"context"
	"os"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	cloudflareAPITokenEnvironmentVariable = "CLOUDFLARE_API_TOKEN"
	cloudflareProductionEndpoint          = "https://api.cloudflare.com/client/v4"
)

var _ provider.Provider = &cfdevProvider{}

// New returns the provider factory consumed by the Terraform provider server.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cfdevProvider{
			version:       version,
			clientFactory: newCloudflareClient,
		}
	}
}

type cfdevProvider struct {
	version       string
	clientFactory cloudflareClientFactory
}

type providerModel struct {
	CloudflareAPIToken    types.String `tfsdk:"cloudflare_api_token"`
	CloudflareAPIEndpoint types.String `tfsdk:"cloudflare_api_endpoint"`
}

type cloudflareClient struct {
	API *cloudflare.API
}

type cloudflareClientFactory func(context.Context, string, string) (*cloudflareClient, error)

func newCloudflareClient(_ context.Context, endpoint, token string) (*cloudflareClient, error) {
	client, err := cloudflare.NewWithAPIToken(token, cloudflare.BaseURL(endpoint))
	if err != nil {
		return nil, err
	}
	return &cloudflareClient{API: client}, nil
}

func (p *cfdevProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cfdev"
	resp.Version = p.version
}

func (p *cfdevProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The cfdev provider manages Cloudflare developer resources.",
		Attributes: map[string]schema.Attribute{
			"cloudflare_api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Cloudflare API token. May also be set with the `CLOUDFLARE_API_TOKEN` environment variable.",
			},
			"cloudflare_api_endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cloudflare API endpoint. Defaults to the production Cloudflare API endpoint.",
			},
		},
	}
}

func (p *cfdevProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.CloudflareAPIToken.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("cloudflare_api_token"),
			"Unknown Cloudflare API Token",
			"The provider cannot connect because cloudflare_api_token is unknown. Set a static value or use the CLOUDFLARE_API_TOKEN environment variable.",
		)
		return
	}
	if config.CloudflareAPIEndpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("cloudflare_api_endpoint"),
			"Unknown Cloudflare API Endpoint",
			"The provider cannot connect because cloudflare_api_endpoint is unknown. Set a static endpoint value.",
		)
		return
	}

	token := config.CloudflareAPIToken.ValueString()
	if token == "" {
		token = os.Getenv(cloudflareAPITokenEnvironmentVariable)
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"No Cloudflare API Token Found",
			"Set cloudflare_api_token in the provider configuration or set the CLOUDFLARE_API_TOKEN environment variable.",
		)
		return
	}

	endpoint := config.CloudflareAPIEndpoint.ValueString()
	if endpoint == "" {
		endpoint = cloudflareProductionEndpoint
	}

	factory := p.clientFactory
	if factory == nil {
		factory = newCloudflareClient
	}
	client, err := factory(ctx, endpoint, token)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Cloudflare API Client",
			"The provider could not create a Cloudflare API client. Verify the configured endpoint and API token.",
		)
		return
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *cfdevProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *cfdevProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
