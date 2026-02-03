package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func dataSourceRecord() *schema.Resource {
	return &schema.Resource{
		Description: "Reads an existing DNS record via samba-tool.",

		ReadContext: dataSourceRecordRead,

		Schema: map[string]*schema.Schema{
			"dns_server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "DNS server hostname (e.g., dns.example.com).",
			},
			"zone": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "DNS zone name (e.g., example.com).",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Record name to look up.",
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
				ValidateFunc: validation.StringInSlice([]string{
					"A", "AAAA", "CNAME", "TXT", "MX", "PTR", "SRV", "NS",
				}, true),
				StateFunc:   func(v interface{}) string { return strings.ToUpper(v.(string)) },
				Description: "Record type (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS).",
			},
			// Computed attributes
			"value": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The record value.",
			},
			"ttl": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "Time to live in seconds.",
			},
		},
	}
}

func dataSourceRecordRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*apiClient).client

	server := d.Get("dns_server").(string)
	zone := d.Get("zone").(string)
	name := d.Get("name").(string)
	recordType := strings.ToUpper(d.Get("type").(string))

	record, err := c.QueryRecord(server, zone, name, recordType)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to query record: %w", err))
	}

	if record == nil {
		return diag.Errorf("record not found: %s %s in zone %s", name, recordType, zone)
	}

	d.SetId(buildID(server, zone, name, recordType))
	d.Set("value", record.Value)
	d.Set("ttl", record.TTL)

	return nil
}
