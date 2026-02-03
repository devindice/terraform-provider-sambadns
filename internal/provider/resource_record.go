package provider

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// normalizeIPv6 expands an IPv6 address to its full form for comparison
// e.g., "2001:db8::1" -> "2001:0db8:0000:0000:0000:0000:0000:0001"
func normalizeIPv6(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip // Return as-is if not a valid IP
	}
	// Check if it's IPv6 (To16 returns 16 bytes for both, but To4 returns nil for IPv6)
	if parsed.To4() != nil {
		return ip // It's IPv4, return as-is
	}
	// Expand to full IPv6 format
	ipv6 := parsed.To16()
	if ipv6 == nil {
		return ip
	}
	// Format as 8 groups of 4 hex digits
	return fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		ipv6[0], ipv6[1], ipv6[2], ipv6[3],
		ipv6[4], ipv6[5], ipv6[6], ipv6[7],
		ipv6[8], ipv6[9], ipv6[10], ipv6[11],
		ipv6[12], ipv6[13], ipv6[14], ipv6[15])
}

// suppressValueDiff handles format differences between config and DNS server response
// - AAAA: IPv6 short form vs expanded form
// - CNAME: with/without trailing dot (FQDN format)
func suppressValueDiff(k, old, new string, d *schema.ResourceData) bool {
	recordType := strings.ToUpper(d.Get("type").(string))
	
	switch recordType {
	case "AAAA":
		return normalizeIPv6(old) == normalizeIPv6(new)
	case "CNAME":
		// Normalize trailing dots - DNS returns FQDN with dot, users often omit it
		oldNorm := strings.TrimSuffix(old, ".")
		newNorm := strings.TrimSuffix(new, ".")
		return oldNorm == newNorm
	default:
		return false
	}
}

func resourceRecord() *schema.Resource {
	return &schema.Resource{
		Description: "Manages a DNS record via samba-tool (MS-DNSP RPC). Supports wildcard records.",

		CreateContext: resourceRecordCreate,
		ReadContext:   resourceRecordRead,
		UpdateContext: resourceRecordUpdate,
		DeleteContext: resourceRecordDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"dns_server": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "DNS server hostname (e.g., dns.example.com).",
			},
			"zone": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "DNS zone name (e.g., example.com).",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Record name. Use * for wildcards (e.g., *.myapp, *.sub.myapp).",
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
				ValidateFunc: validation.StringInSlice([]string{
					"A", "AAAA", "CNAME", "TXT", "MX", "PTR", "SRV", "NS",
				}, true),
				StateFunc:   func(v interface{}) string { return strings.ToUpper(v.(string)) },
				Description: "Record type (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS).",
			},
			"value": {
				Type:             schema.TypeString,
				Required:         true,
				DiffSuppressFunc: suppressValueDiff,
				Description:      "Record value. For A: IP address, CNAME: FQDN, MX: priority hostname, etc.",
			},
			"ttl": {
				Type:        schema.TypeInt,
				Optional:    true,
				Computed:    true,
				Description: "Time to live in seconds. Defaults to zone default (typically 3600).",
			},
		},
	}
}

// buildID creates a unique resource ID
func buildID(server, zone, name, recordType string) string {
	return fmt.Sprintf("%s/%s/%s/%s", server, zone, name, strings.ToUpper(recordType))
}

// parseID extracts components from resource ID
func parseID(id string) (server, zone, name, recordType string, err error) {
	parts := strings.SplitN(id, "/", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("invalid ID format: %s (expected server/zone/name/type)", id)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func resourceRecordCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*apiClient).client

	record := DNSRecord{
		Server: d.Get("dns_server").(string),
		Zone:   d.Get("zone").(string),
		Name:   d.Get("name").(string),
		Type:   strings.ToUpper(d.Get("type").(string)),
		Value:  d.Get("value").(string),
	}

	if err := c.CreateRecord(record); err != nil {
		return diag.FromErr(fmt.Errorf("failed to create record: %w", err))
	}

	d.SetId(buildID(record.Server, record.Zone, record.Name, record.Type))

	// Read back to get computed values like TTL
	return resourceRecordRead(ctx, d, m)
}

func resourceRecordRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*apiClient).client

	server, zone, name, recordType, err := parseID(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	record, err := c.QueryRecord(server, zone, name, recordType)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to query record: %w", err))
	}

	if record == nil {
		// Record doesn't exist, remove from state
		d.SetId("")
		return nil
	}

	d.Set("dns_server", record.Server)
	d.Set("zone", record.Zone)
	d.Set("name", record.Name)
	d.Set("type", record.Type)
	d.Set("value", record.Value)
	d.Set("ttl", record.TTL)

	return nil
}

func resourceRecordUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*apiClient).client

	if d.HasChange("value") {
		server := d.Get("dns_server").(string)
		zone := d.Get("zone").(string)
		name := d.Get("name").(string)
		recordType := strings.ToUpper(d.Get("type").(string))
		newValue := d.Get("value").(string)

		// Query current record to get actual stored value for deletion
		current, err := c.QueryRecord(server, zone, name, recordType)
		if err != nil {
			return diag.FromErr(fmt.Errorf("failed to query record for update: %w", err))
		}

		// Delete old record using actual stored value
		if current != nil {
			oldRecord := DNSRecord{
				Server: server,
				Zone:   zone,
				Name:   name,
				Type:   recordType,
				Value:  current.Value,
			}
			if err := c.DeleteRecord(oldRecord); err != nil {
				return diag.FromErr(fmt.Errorf("failed to delete old record: %w", err))
			}
		}

		// Create new record
		newRecord := DNSRecord{
			Server: server,
			Zone:   zone,
			Name:   name,
			Type:   recordType,
			Value:  newValue,
		}
		if err := c.CreateRecord(newRecord); err != nil {
			return diag.FromErr(fmt.Errorf("failed to create new record: %w", err))
		}
	}

	return resourceRecordRead(ctx, d, m)
}

func resourceRecordDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*apiClient).client

	server := d.Get("dns_server").(string)
	zone := d.Get("zone").(string)
	name := d.Get("name").(string)
	recordType := strings.ToUpper(d.Get("type").(string))

	// Query current record to get the actual stored value (may differ from config)
	current, err := c.QueryRecord(server, zone, name, recordType)
	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to query record for deletion: %w", err))
	}

	// If record doesn't exist, nothing to delete
	if current == nil {
		d.SetId("")
		return nil
	}

	// Delete using the actual stored value
	record := DNSRecord{
		Server: server,
		Zone:   zone,
		Name:   name,
		Type:   recordType,
		Value:  current.Value,
	}

	if err := c.DeleteRecord(record); err != nil {
		return diag.FromErr(fmt.Errorf("failed to delete record: %w", err))
	}

	d.SetId("")
	return nil
}
