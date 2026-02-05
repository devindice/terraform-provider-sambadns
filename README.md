# Terraform Provider for samba-tool DNS (sambadns)

[![Registry](https://img.shields.io/badge/terraform-registry-blueviolet)](https://registry.terraform.io/providers/devindice/sambadns/latest)
[![Release](https://img.shields.io/github/v/release/devindice/terraform-provider-sambadns)](https://github.com/devindice/terraform-provider-sambadns/releases)
[![License](https://img.shields.io/github/license/devindice/terraform-provider-sambadns)](LICENSE)

This Terraform provider allows you to manage DNS records on Windows DNS servers via **samba-tool** using the MS-DNSP RPC protocol. It supports A, AAAA, CNAME, TXT, MX, PTR, SRV, and NS records, including **wildcard records** that RFC 2136 cannot handle.

## Prerequisites

- **Terraform** >= 1.0
- **samba-tool** installed (part of `samba-common-bin` on Debian/Ubuntu)
- **Network access** to Windows DC (MS-DNSP RPC, typically port 135 + dynamic)
- **AD credentials** with DNS management permissions

## Why use this provider?

Existing Terraform providers for Windows DNS (like `portofportland/windns`) use WinRM/PowerShell, which serializes all operations through a single connection—limiting throughput to ~5 records/minute regardless of parallelism settings. With large DNS deployments, `terraform plan` can take hours.

The `hashicorp/dns` provider using RFC 2136 is fast (~2400 rec/min), but Windows DNS **intentionally blocks wildcard record creation via RFC 2136** dynamic updates, regardless of authentication method. This is a Windows implementation limitation, not a permissions issue.

samba-tool speaks **MS-DNSP RPC directly from Linux**—the same protocol PowerShell uses under the hood, but without the WinRM/SSH bottleneck. Each Terraform worker spawns its own samba-tool process, enabling true parallel execution (~730 rec/min with `-parallelism=10`). No SSH or WinRM needed.

| Approach | Protocol | Wildcards | Speed | Notes |
|----------|----------|-----------|-------|-------|
| portofportland/windns | WinRM/PowerShell | Yes | ~5 rec/min | Single connection serializes all operations |
| techbuzz/windns | SSH/PowerShell | Yes | ~5 rec/min | Same serialization issue over SSH |
| hashicorp/dns (RFC 2136) | DNS UPDATE | No | ~2400 rec/min | Windows blocks wildcards at protocol level |
| Direct LDAP writes | LDAP | Yes | Unknown | Records created but DNS didn't serve them |
| GSSAPI/GSS-TSIG | DNS UPDATE + Kerberos | No | Fast | Same wildcard limitation as RFC 2136 |
| **samba-tool (this provider)** | MS-DNSP RPC | **Yes** | ~730 rec/min | True parallelism, no bottleneck |

## Key Features

- **Full CRUD support** for DNS records (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS)
- **Wildcard record support** (e.g., `*.myapp.example.com`)
- **Drift detection** - detects external changes and corrects on apply
- **Data source** for reading existing records
- **Parallel execution** - supports Terraform's `-parallelism` flag
- **Value normalization** - handles IPv6 expansion, CNAME trailing dots automatically

---

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

---

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

### Authentication Format

- Username must include realm: `user@REALM.COM` (uppercase realm)
- Password passed via `--password="..."` flag internally

---

## Resource: sambadns_record

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `dns_server` | string | Yes | DNS server hostname (the DC) |
| `zone` | string | Yes | DNS zone name |
| `name` | string | Yes | Record name (`@` for apex, `*` for wildcards) |
| `type` | string | Yes | Record type (A, AAAA, CNAME, TXT, MX, PTR, SRV, NS) |
| `value` | string | Yes | Record value (format varies by type) |

### Attributes (Read-only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Resource ID format: `server/zone/name/type` |
| `ttl` | int | Time to live (read from DNS server) |

---

## Examples

### A Record

```hcl
resource "sambadns_record" "web" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "web"
  type       = "A"
  value      = "192.168.1.100"
}
```

### CNAME Record

```hcl
resource "sambadns_record" "www" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "www"
  type       = "CNAME"
  value      = "web.example.com"
}
```

### Wildcard CNAME

```hcl
resource "sambadns_record" "wildcard" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "*.myapp"
  type       = "CNAME"
  value      = "loadbalancer.example.com"
}
```

This creates a wildcard that matches:
- `foo.myapp.example.com`
- `bar.myapp.example.com`
- `anything.myapp.example.com`

### AAAA Record (IPv6)

```hcl
resource "sambadns_record" "ipv6" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "ipv6host"
  type       = "AAAA"
  value      = "2001:db8::1"  # Short form OK, auto-normalized
}
```

### MX Record

```hcl
resource "sambadns_record" "mail" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "@"
  type       = "MX"
  value      = "mail.example.com 10"  # Format: "hostname priority"
}
```

### TXT Record

```hcl
resource "sambadns_record" "spf" {
  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = "@"
  type       = "TXT"
  value      = "v=spf1 include:_spf.google.com ~all"
}
```

### Multiple Records with for_each

```hcl
locals {
  a_records = {
    "web" = "192.168.1.100"
    "api" = "192.168.1.101"
    "db"  = "192.168.1.102"
  }
}

resource "sambadns_record" "servers" {
  for_each = local.a_records

  dns_server = "dc01.example.com"
  zone       = "example.com"
  name       = each.key
  type       = "A"
  value      = each.value
}
```

---

## Data Source: sambadns_record

Read existing DNS records without managing them.

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

---

## Import

Existing records can be imported:

```bash
terraform import sambadns_record.web "dc01.example.com/example.com/web/A"
terraform import sambadns_record.wildcard "dc01.example.com/example.com/*.myapp/CNAME"
```

---

## Performance

Tested with `-parallelism=10`:

| Operation | 100 Records | 1000 Records |
|-----------|-------------|--------------|
| Create | 10s (~10/s) | 82s (~12/s) |
| Plan | 2s | 39s |
| Destroy | 12s (~8/s) | 117s (~8.5/s) |

### Tips

- Use `-parallelism=10` or higher for bulk operations
- Use `for_each` over `count` for better state management

---

## Record Type Notes

### MX Records
Value format: `hostname priority` (e.g., `mail.example.com 10`)

### TXT Records
Long TXT records (>255 chars) are automatically split and reassembled.

### AAAA Records
IPv6 addresses can be specified in short form. The provider normalizes addresses to prevent drift.

### CNAME Records
Trailing dots are handled automatically (`target.example.com` and `target.example.com.` are equivalent).

---

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

### Drift Detection

The provider queries DNS on every plan to detect external changes. If records are modified outside Terraform, the next plan will show the required changes.

---

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MPL-2.0 - see [LICENSE](LICENSE) for details.
