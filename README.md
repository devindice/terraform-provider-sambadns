# Terraform Provider for samba-tool DNS (sambadns)

[![Registry](https://img.shields.io/badge/terraform-registry-blueviolet)](https://registry.terraform.io/providers/devindice/sambadns/latest)
[![Release](https://img.shields.io/github/v/release/devindice/terraform-provider-sambadns)](https://github.com/devindice/terraform-provider-sambadns/releases)
[![License](https://img.shields.io/github/license/devindice/terraform-provider-sambadns)](LICENSE)

A Terraform provider for managing Windows DNS records via samba-tool (MS-DNSP RPC). This provider enables management of DNS records on Windows DNS servers from Linux hosts, including support for **wildcard records** that RFC 2136 (nsupdate) cannot handle.

## Features

- **Full CRUD support** for DNS records (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS)
- **Wildcard record support** (e.g., `*.myapp.example.com`)
- **Drift detection** - detects external changes and corrects on apply
- **Data source** for reading existing records
- **Parallel execution** - supports Terraform's `-parallelism` flag

## Requirements

- Terraform >= 1.0
- samba-tool installed on the machine running Terraform
- Network access to the Windows DNS server (MS-DNSP RPC)
- Valid AD credentials with DNS management permissions

## Installation

### Terraform Registry (Recommended)

```hcl
terraform {
  required_providers {
    sambadns = {
      source  = "devindice/sambadns"
      version = "~> 1.0"
    }
  }
}
```

Then run `terraform init`.

### Manual Installation

Download the appropriate binary from [Releases](https://github.com/devindice/terraform-provider-sambadns/releases) and place it in:

```
~/.terraform.d/plugins/registry.terraform.io/devindice/sambadns/<VERSION>/<OS>_<ARCH>/
```

## Provider Configuration

```hcl
provider "sambadns" {
  username = "terraform@EXAMPLE.COM"    # AD username with DNS admin rights
  password = var.sambadns_password       # Password (use env: SAMBADNS_PASSWORD)
}
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SAMBADNS_USERNAME` | AD username (alternative to config) |
| `SAMBADNS_PASSWORD` | AD password (recommended over config) |

## Resources

### sambadns_record

Manages a DNS record via samba-tool.

```hcl
# A Record
resource "sambadns_record" "web" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "web"
  type       = "A"
  value      = "192.168.1.100"
}

# CNAME Record
resource "sambadns_record" "www" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "www"
  type       = "CNAME"
  value      = "web.example.com"
}

# Wildcard CNAME
resource "sambadns_record" "wildcard" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "*.myapp"
  type       = "CNAME"
  value      = "loadbalancer.example.com"
}

# AAAA Record (IPv6)
resource "sambadns_record" "ipv6" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "ipv6host"
  type       = "AAAA"
  value      = "2001:db8::1"
}

# MX Record
resource "sambadns_record" "mail" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "@"
  type       = "MX"
  value      = "mail.example.com 10"  # hostname priority
}

# TXT Record
resource "sambadns_record" "spf" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "@"
  type       = "TXT"
  value      = "v=spf1 include:_spf.google.com ~all"
}
```

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `dns_server` | string | Yes | DNS server hostname (the DC) |
| `zone` | string | Yes | DNS zone name |
| `name` | string | Yes | Record name (use `*` for wildcards, `@` for apex) |
| `type` | string | Yes | Record type (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS) |
| `value` | string | Yes | Record value |

#### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource ID (`server/zone/name/type`) |
| `ttl` | int | Time to live (read from DNS server) |

## Data Sources

### sambadns_record

Reads an existing DNS record.

```hcl
data "sambadns_record" "existing" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "web"
  type       = "A"
}

output "web_ip" {
  value = data.sambadns_record.existing.value
}
```

## Import

Existing records can be imported:

```bash
terraform import sambadns_record.web "dc01.example.com/example.com/web/A"
terraform import sambadns_record.wildcard "dc01.example.com/example.com/*.myapp/CNAME"
```

## Record Type Notes

### MX Records
Value format: `hostname priority` (e.g., `mail.example.com 10`)

### TXT Records
Multi-part TXT records are handled automatically.

### AAAA Records
IPv6 addresses can be specified in short form (e.g., `2001:db8::1`). The provider normalizes addresses to prevent drift.

### CNAME Records
Trailing dots are handled automatically (e.g., `target.example.com` and `target.example.com.` are equivalent).

## Troubleshooting

### Authentication Errors

```
NT_STATUS_LOGON_FAILURE
```

- Verify username format includes realm: `user@REALM.COM` (uppercase)
- Check if account is locked
- Ensure network access to DC

### Record Already Exists

The provider is idempotent - if a record already exists with the same value, no error is raised. If the value differs, an error is returned.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MPL-2.0 - see [LICENSE](LICENSE) for details.
