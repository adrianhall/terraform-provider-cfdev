package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	p := New("test")()
	response := &frameworkprovider.MetadataResponse{}
	p.Metadata(context.Background(), frameworkprovider.MetadataRequest{}, response)

	if response.TypeName != "cfdev" {
		t.Errorf("TypeName = %q, want %q", response.TypeName, "cfdev")
	}
	if response.Version != "test" {
		t.Errorf("Version = %q, want %q", response.Version, "test")
	}
}

func TestProviderFactoryReturnsProvider(t *testing.T) {
	t.Parallel()

	if got := New("test")(); got == nil {
		t.Fatal("New()() = nil, want provider")
	}
}

func TestProviderSchema(t *testing.T) {
	t.Parallel()

	p := New("test")()
	response := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), frameworkprovider.SchemaRequest{}, response)

	if response.Schema.Description == "" {
		t.Error("Schema description is empty")
	}

	token, ok := response.Schema.Attributes["cloudflare_api_token"]
	if !ok {
		t.Fatal("cloudflare_api_token is not defined")
	}
	if !token.IsSensitive() {
		t.Error("cloudflare_api_token is not sensitive")
	}
	if !token.IsOptional() {
		t.Error("cloudflare_api_token is not optional")
	}
	if !response.Schema.Attributes["cloudflare_api_endpoint"].IsOptional() {
		t.Error("cloudflare_api_endpoint is not optional")
	}
}

func TestProviderConfigureUsesProductionEndpointAndConfiguredToken(t *testing.T) {
	t.Parallel()

	var receivedEndpoint, receivedToken string
	p := &cfdevProvider{
		clientFactory: func(_ context.Context, endpoint, token string) (*cloudflareClient, error) {
			receivedEndpoint, receivedToken = endpoint, token
			return &cloudflareClient{}, nil
		},
	}
	response := configure(t, p, map[string]tftypes.Value{
		"cloudflare_api_token":    tftypes.NewValue(tftypes.String, "configured-token"),
		"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, nil),
	})

	if response.Diagnostics.HasError() {
		t.Fatalf("Configure() diagnostics = %v", response.Diagnostics)
	}
	if receivedEndpoint != cloudflareProductionEndpoint {
		t.Errorf("endpoint = %q, want %q", receivedEndpoint, cloudflareProductionEndpoint)
	}
	if receivedToken != "configured-token" {
		t.Errorf("token = %q, want configured token", receivedToken)
	}
	if response.ResourceData == nil || response.DataSourceData == nil {
		t.Error("configured client was not shared with resources and data sources")
	}
}

func TestProviderConfigureUsesAlternateEndpointAndEnvironmentToken(t *testing.T) {
	t.Setenv(cloudflareAPITokenEnvironmentVariable, "environment-token")

	var receivedEndpoint, receivedToken string
	p := &cfdevProvider{
		clientFactory: func(_ context.Context, endpoint, token string) (*cloudflareClient, error) {
			receivedEndpoint, receivedToken = endpoint, token
			return &cloudflareClient{}, nil
		},
	}
	response := configure(t, p, map[string]tftypes.Value{
		"cloudflare_api_token":    tftypes.NewValue(tftypes.String, nil),
		"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, "https://api.example.test/client/v4"),
	})

	if response.Diagnostics.HasError() {
		t.Fatalf("Configure() diagnostics = %v", response.Diagnostics)
	}
	if receivedEndpoint != "https://api.example.test/client/v4" {
		t.Errorf("endpoint = %q, want alternate endpoint", receivedEndpoint)
	}
	if receivedToken != "environment-token" {
		t.Errorf("token = %q, want environment token", receivedToken)
	}
}

func TestProviderConfigureDoesNotExposeTokenInDiagnostics(t *testing.T) {
	t.Parallel()

	const token = "token-that-must-not-leak"
	p := &cfdevProvider{
		clientFactory: func(_ context.Context, _, _ string) (*cloudflareClient, error) {
			return nil, errors.New(token)
		},
	}
	response := configure(t, p, map[string]tftypes.Value{
		"cloudflare_api_token":    tftypes.NewValue(tftypes.String, token),
		"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, nil),
	})

	if !response.Diagnostics.HasError() {
		t.Fatal("Configure() produced no error diagnostics")
	}
	if strings.Contains(diagnosticsString(response.Diagnostics), token) {
		t.Errorf("diagnostics expose API token: %s", diagnosticsString(response.Diagnostics))
	}
}

func TestNewCloudflareClient(t *testing.T) {
	t.Parallel()

	client, err := newCloudflareClient(context.Background(), "https://api.example.test/client/v4", "test-token")
	if err != nil {
		t.Fatalf("newCloudflareClient() error = %v", err)
	}
	if client.API.BaseURL != "https://api.example.test/client/v4" {
		t.Errorf("BaseURL = %q, want alternate endpoint", client.API.BaseURL)
	}
}

func TestProviderConfigureRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	for name, values := range map[string]map[string]tftypes.Value{
		"token": {
			"cloudflare_api_token":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, nil),
		},
		"endpoint": {
			"cloudflare_api_token":    tftypes.NewValue(tftypes.String, "configured-token"),
			"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := configure(t, &cfdevProvider{}, values)
			if !response.Diagnostics.HasError() {
				t.Fatal("Configure() produced no error diagnostics")
			}
		})
	}
}

func TestProviderConfigureRequiresToken(t *testing.T) {
	t.Setenv(cloudflareAPITokenEnvironmentVariable, "")

	p := &cfdevProvider{}
	response := configure(t, p, map[string]tftypes.Value{
		"cloudflare_api_token":    tftypes.NewValue(tftypes.String, nil),
		"cloudflare_api_endpoint": tftypes.NewValue(tftypes.String, nil),
	})

	if !response.Diagnostics.HasError() {
		t.Fatal("Configure() produced no error diagnostics")
	}
	if !strings.Contains(diagnosticsString(response.Diagnostics), cloudflareAPITokenEnvironmentVariable) {
		t.Errorf("diagnostics = %s, want environment-variable guidance", diagnosticsString(response.Diagnostics))
	}
}

func TestProviderRegistrations(t *testing.T) {
	t.Parallel()

	p := New("test")()
	if resources := p.Resources(context.Background()); resources != nil {
		t.Errorf("Resources() = %v, want nil", resources)
	}
	if dataSources := p.DataSources(context.Background()); dataSources != nil {
		t.Errorf("DataSources() = %v, want nil", dataSources)
	}
}

func configure(t *testing.T, p *cfdevProvider, values map[string]tftypes.Value) *frameworkprovider.ConfigureResponse {
	t.Helper()

	schemaResponse := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), frameworkprovider.SchemaRequest{}, schemaResponse)
	response := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), frameworkprovider.ConfigureRequest{Config: tfsdk.Config{
		Raw:    tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{"cloudflare_api_token": tftypes.String, "cloudflare_api_endpoint": tftypes.String}}, values),
		Schema: schemaResponse.Schema,
	}}, response)
	return response
}

func diagnosticsString(diagnostics diag.Diagnostics) string {
	return diagnostics.Errors()[0].Detail()
}
