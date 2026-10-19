package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	p := New("test")()
	response := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, response)

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

func TestProviderSchemaAndRegistrations(t *testing.T) {
	t.Parallel()

	p := New("test")()
	schemaResponse := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResponse)
	if schemaResponse.Schema.Description == "" {
		t.Error("Schema description is empty")
	}

	p.Configure(context.Background(), provider.ConfigureRequest{}, &provider.ConfigureResponse{})
	if resources := p.Resources(context.Background()); resources != nil {
		t.Errorf("Resources() = %v, want nil", resources)
	}
	if dataSources := p.DataSources(context.Background()); dataSources != nil {
		t.Errorf("DataSources() = %v, want nil", dataSources)
	}
}
