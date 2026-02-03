package provider

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// SambaClient wraps samba-tool DNS operations
type SambaClient struct {
	Username string
	Password string
}

// DNSRecord represents a DNS record
type DNSRecord struct {
	Server string
	Zone   string
	Name   string
	Type   string
	Value  string
	TTL    int
}

// NewSambaClient creates a new samba-tool client
func NewSambaClient(username, password string) *SambaClient {
	return &SambaClient{
		Username: username,
		Password: password,
	}
}

// authArgs returns the authentication arguments for samba-tool
func (c *SambaClient) authArgs() []string {
	return []string{"-U", fmt.Sprintf("%s%%%s", c.Username, c.Password)}
}

// runCommand executes samba-tool with the given arguments
func (c *SambaClient) runCommand(args ...string) (string, error) {
	fullArgs := append(args, c.authArgs()...)
	cmd := exec.Command("samba-tool", fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include stderr in error message for debugging
		return "", fmt.Errorf("samba-tool error: %v, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// CreateRecord creates a DNS record
func (c *SambaClient) CreateRecord(r DNSRecord) error {
	args := []string{"dns", "add", r.Server, r.Zone, r.Name, r.Type, r.Value}
	_, err := c.runCommand(args...)
	if err != nil {
		// Check if record already exists
		if strings.Contains(err.Error(), "already exist") {
			// Record exists - check if value matches
			existing, queryErr := c.QueryRecord(r.Server, r.Zone, r.Name, r.Type)
			if queryErr == nil && existing != nil && existing.Value == r.Value {
				// Same value, idempotent success
				return nil
			}
			return fmt.Errorf("record already exists with different value")
		}
		return err
	}
	return nil
}

// QueryRecord reads a DNS record
func (c *SambaClient) QueryRecord(server, zone, name, recordType string) (*DNSRecord, error) {
	args := []string{"dns", "query", server, zone, name, recordType}
	output, err := c.runCommand(args...)
	if err != nil {
		if strings.Contains(err.Error(), "WERR_DNS_ERROR_NAME_DOES_NOT_EXIST") ||
			strings.Contains(err.Error(), "WERR_DNS_ERROR_RECORD_DOES_NOT_EXIST") ||
			strings.Contains(err.Error(), "does not exist") {
			return nil, nil // Record does not exist
		}
		return nil, err
	}

	record, parseErr := parseQueryOutput(output, server, zone, name, recordType)
	if parseErr != nil {
		return nil, parseErr
	}
	if record != nil {
	}
	return record, nil
}

// formatTXTForDelete converts TXT value from query format to delete format
// Query returns: "string1","string2"
// Delete needs:  'string1' 'string2'
func formatTXTForDelete(value string) string {
	// Split on ","
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		// Remove surrounding double quotes and add single quotes
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		result = append(result, "'"+part+"'")
	}
	return strings.Join(result, " ")
}

// DeleteRecord removes a DNS record
func (c *SambaClient) DeleteRecord(r DNSRecord) error {
	value := r.Value

	// TXT records need special formatting for delete
	if strings.ToUpper(r.Type) == "TXT" && strings.Contains(value, ",") {
		value = formatTXTForDelete(value)
	}

	args := []string{"dns", "delete", r.Server, r.Zone, r.Name, r.Type, value}


	_, err := c.runCommand(args...)
	if err != nil {
		// If record doesn't exist, treat as success
		if strings.Contains(err.Error(), "WERR_DNS_ERROR_NAME_DOES_NOT_EXIST") ||
			strings.Contains(err.Error(), "WERR_DNS_ERROR_RECORD_DOES_NOT_EXIST") ||
			strings.Contains(err.Error(), "does not exist") {
			return nil
		}
		return err
	}
	return nil
}

// UpdateRecord updates a DNS record (delete + create)
func (c *SambaClient) UpdateRecord(old, new DNSRecord) error {
	// Delete old record
	if err := c.DeleteRecord(old); err != nil {
		return fmt.Errorf("failed to delete old record: %w", err)
	}

	// Create new record
	if err := c.CreateRecord(new); err != nil {
		return fmt.Errorf("failed to create new record: %w", err)
	}

	return nil
}

// parseQueryOutput parses samba-tool dns query output
// Example output:
//
//	Name=*, Records=1, Children=0
//	  CNAME: target.example.com (flags=600000f0, serial=123, ttl=3600)
//	  MX: mail.example.com. (10) (flags=f0, serial=0, ttl=900)
func parseQueryOutput(output, server, zone, name, recordType string) (*DNSRecord, error) {
	lines := strings.Split(output, "\n")

	// Look for the record type line
	typePrefix := fmt.Sprintf("%s:", strings.ToUpper(recordType))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, typePrefix) {
			// Parse: "CNAME: value (flags=..., serial=..., ttl=3600)"
			// or "A: 192.168.1.1 (flags=..., serial=..., ttl=3600)"
			// or "MX: mail.example.com. (10) (flags=f0, serial=0, ttl=900)"

			// Extract value (between type: and opening paren)
			afterType := strings.TrimPrefix(line, typePrefix)
			afterType = strings.TrimSpace(afterType)

			parenIdx := strings.Index(afterType, "(")
			if parenIdx == -1 {
				return nil, fmt.Errorf("unexpected output format: %s", line)
			}

			value := strings.TrimSpace(afterType[:parenIdx])

			// For MX records, extract priority from first (N) and append to value
			// Format: "mail.example.com. (10) (flags=...)"
			// Priority is the first parenthesized number
			if strings.ToUpper(recordType) == "MX" {
				// Match first (N) which is the priority
				priRegex := regexp.MustCompile(`^\((\d+)\)`)
				remaining := strings.TrimSpace(afterType[parenIdx:])
				if matches := priRegex.FindStringSubmatch(remaining); len(matches) > 1 {
					priority := matches[1]
					// Remove trailing dot from hostname if present
					value = strings.TrimSuffix(value, ".")
					// Format: "hostname priority" for samba-tool delete
					value = fmt.Sprintf("%s %s", value, priority)
				}
			}

			// Extract TTL
			ttl := 3600 // default
			ttlRegex := regexp.MustCompile(`ttl=(\d+)`)
			if matches := ttlRegex.FindStringSubmatch(afterType); len(matches) > 1 {
				if parsed, err := strconv.Atoi(matches[1]); err == nil {
					ttl = parsed
				}
			}

			return &DNSRecord{
				Server: server,
				Zone:   zone,
				Name:   name,
				Type:   strings.ToUpper(recordType),
				Value:  value,
				TTL:    ttl,
			}, nil
		}
	}

	return nil, fmt.Errorf("record type %s not found in output", recordType)
}
