package types

import (
	"context"
	"time"
)

// ProviderName identifies a cloud provider.
type ProviderName string

const (
	ProviderDigitalOcean ProviderName = "digitalocean"
	ProviderHetzner      ProviderName = "hetzner"
)

func SupportedProviders() []ProviderName {
	return []ProviderName{ProviderDigitalOcean, ProviderHetzner}
}

// DefaultImage returns the default Ubuntu 24.04 image slug for each provider.
func DefaultImage(provider ProviderName) string {
	switch provider {
	case ProviderDigitalOcean:
		return "ubuntu-24-04-x64"
	case ProviderHetzner:
		return "ubuntu-24.04"
	default:
		return ""
	}
}

// Provider defines the interface for cloud server providers.
type Provider interface {
	Name() string
	DisplayName() string

	CreateServer(ctx context.Context, opts CreateServerOptions) (*Server, error)
	GetServer(ctx context.Context, id string) (*Server, error)
	GetServers(ctx context.Context) ([]Server, error)
	WaitForServer(ctx context.Context, id string, timeout time.Duration) (*Server, error)

	GetRegions(ctx context.Context) ([]Region, error)
	GetSizes(ctx context.Context, region string) ([]Size, error)

	GetSSHKey(ctx context.Context, fingerprint string) (*SSHKey, error)
	CreateSSHKey(ctx context.Context, name string, publicKey string) (*SSHKey, error)
}

// DNSProvider is optionally implemented by providers supporting DNS management.
type DNSProvider interface {
	CreateZone(ctx context.Context, domain string) error
	GetZone(ctx context.Context, domain string) (*Zone, bool, error)
	CreateRecord(ctx context.Context, domain string, record DNSRecord) (*DNSRecord, error)
	DeleteRecord(ctx context.Context, domain string, recordID string) error
	ListRecords(ctx context.Context, domain string) ([]DNSRecord, error)
}

// ProviderWithDNS combines both interfaces.
type ProviderWithDNS interface {
	Provider
	DNSProvider
}

// ServerStatus represents the state of a server.
type ServerStatus string

const (
	ServerStatusPending  ServerStatus = "pending"
	ServerStatusStarting ServerStatus = "starting"
	ServerStatusRunning  ServerStatus = "running"
	ServerStatusStopped  ServerStatus = "stopped"
	ServerStatusError    ServerStatus = "error"
	ServerStatusUnknown  ServerStatus = "unknown"
)

// Server represents a cloud server instance.
type Server struct {
	ID           string
	Name         string
	Status       ServerStatus
	PublicIPv4   string
	PublicIPv6   string
	Region       string
	Size         string
	Image        string
	CreatedAt    time.Time
	DashboardURL string
}

// CreateServerOptions contains the parameters for creating a new server.
type CreateServerOptions struct {
	Name      string
	Region    string
	Size      string
	Image     string
	SSHKeyIDs []string
	Tags      map[string]string
}

// Region represents a cloud provider region/location.
type Region struct {
	Slug      string
	Name      string
	Country   string
	Available bool
}

// Size represents a server size/type.
type Size struct {
	Slug         string
	Name         string
	Description  string
	VCPUs        int
	Memory       int // MB
	Disk         int // GB
	Transfer     float64
	PriceHourly  float64
	PriceMonthly float64
	Available    bool
}

// SSHKey represents an SSH public key registered with a provider.
type SSHKey struct {
	ID          string
	Name        string
	Fingerprint string
	PublicKey   string
}

// Zone represents a DNS zone/domain.
type Zone struct {
	ID   string
	Name string
	TTL  int
}

// DNSRecord represents a DNS record.
type DNSRecord struct {
	ID    string
	Type  string
	Name  string
	Value string
	TTL   int
}
