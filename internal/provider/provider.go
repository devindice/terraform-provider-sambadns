package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func init() {
	// Set descriptions to support markdown syntax
	schema.DescriptionKind = schema.StringMarkdown
}

// New returns a new provider
func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"username": {
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("SAMBADNS_USERNAME", nil),
					Description: "Username for samba-tool authentication (e.g., terraform@domain.com). Can also be set via SAMBADNS_USERNAME env var.",
				},
				"password": {
					Type:        schema.TypeString,
					Required:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("SAMBADNS_PASSWORD", nil),
					Description: "Password for samba-tool authentication. Can also be set via SAMBADNS_PASSWORD env var.",
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"sambadns_record": resourceRecord(),
			},
			DataSourcesMap: map[string]*schema.Resource{
				"sambadns_record": dataSourceRecord(),
			},
		}

		p.ConfigureContextFunc = configure(version, p)

		return p
	}
}

// apiClient holds the configured samba client
type apiClient struct {
	client *SambaClient
}

func configure(version string, p *schema.Provider) func(context.Context, *schema.ResourceData) (interface{}, diag.Diagnostics) {
	return func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		username := d.Get("username").(string)
		password := d.Get("password").(string)

		// Allow env vars to override
		if v := os.Getenv("SAMBADNS_USERNAME"); v != "" {
			username = v
		}
		if v := os.Getenv("SAMBADNS_PASSWORD"); v != "" {
			password = v
		}

		if username == "" || password == "" {
			return nil, diag.Errorf("username and password are required")
		}

		client := NewSambaClient(username, password)

		return &apiClient{client: client}, nil
	}
}
