package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/cli"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/ssh"

	"github.com/roots/trellis-cli/pkg/server/digitalocean"
	"github.com/roots/trellis-cli/pkg/server/hetzner"
	"github.com/roots/trellis-cli/pkg/server/types"
)

// Re-export types for convenience
type (
	ProviderName        = types.ProviderName
	Provider            = types.Provider
	DNSProvider         = types.DNSProvider
	ProviderWithDNS     = types.ProviderWithDNS
	Server              = types.Server
	ServerStatus        = types.ServerStatus
	CreateServerOptions = types.CreateServerOptions
	Region              = types.Region
	Size                = types.Size
	SSHKey              = types.SSHKey
	Zone                = types.Zone
	DNSRecord           = types.DNSRecord
)

// Re-export constants
const (
	ProviderDigitalOcean = types.ProviderDigitalOcean
	ProviderHetzner      = types.ProviderHetzner
	ServerStatusPending  = types.ServerStatusPending
	ServerStatusStarting = types.ServerStatusStarting
	ServerStatusRunning  = types.ServerStatusRunning
	ServerStatusStopped  = types.ServerStatusStopped
	ServerStatusError    = types.ServerStatusError
	ServerStatusUnknown  = types.ServerStatusUnknown
)

// Re-export functions
var (
	SupportedProviders = types.SupportedProviders
	DefaultImage       = types.DefaultImage
)

// Token environment variable names for each provider.
var tokenEnvVars = map[ProviderName]string{
	ProviderDigitalOcean: "DIGITALOCEAN_ACCESS_TOKEN",
	ProviderHetzner:      "HCLOUD_TOKEN",
}

// GetProviderToken retrieves the API token for a provider from environment or prompts the user.
func GetProviderToken(provider ProviderName, ui cli.Ui) (string, error) {
	envVar := tokenEnvVars[provider]
	token := os.Getenv(envVar)

	if token == "" {
		ui.Info(fmt.Sprintf("%s environment variable not set.", envVar))
		var err error
		token, err = ui.Ask(fmt.Sprintf("Enter %s API token:", provider))
		if err != nil {
			return "", err
		}
	}

	return token, nil
}

// SSHKeyFingerprint returns the MD5 fingerprint of an SSH public key.
func SSHKeyFingerprint(publicKey ssh.PublicKey) string {
	return ssh.FingerprintLegacyMD5(publicKey)
}

// NewProvider creates a new provider instance based on the provider name.
func NewProvider(name ProviderName, token string) (Provider, error) {
	switch name {
	case ProviderDigitalOcean:
		return digitalocean.New(token), nil
	case ProviderHetzner:
		return hetzner.New(token), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

// NewProviderWithDNS creates a provider that supports DNS management.
func NewProviderWithDNS(name ProviderName, token string) (ProviderWithDNS, error) {
	p, err := NewProvider(name, token)
	if err != nil {
		return nil, err
	}

	pwd, ok := p.(ProviderWithDNS)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support DNS management", name)
	}

	return pwd, nil
}

// DefaultSSHKeyPaths contains the default locations to look for SSH public keys.
var DefaultSSHKeyPaths = []string{"~/.ssh/id_ed25519.pub", "~/.ssh/id_rsa.pub"}

// LoadSSHKey attempts to load an SSH public key from the given paths.
// Returns the path of the loaded key, its contents, and the parsed public key.
func LoadSSHKey(paths []string) (keyPath string, contents []byte, publicKey ssh.PublicKey, err error) {
	for _, path := range paths {
		keyPath = path
		contents, publicKey, err = loadPublicKey(path)
		if err == nil {
			break
		}
	}

	if publicKey == nil {
		return "", nil, nil, fmt.Errorf("no valid SSH public key found. Attempted paths: %s", strings.Join(paths, ", "))
	}

	return keyPath, contents, publicKey, nil
}

func loadPublicKey(path string) ([]byte, ssh.PublicKey, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, nil, err
	}

	key, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	publicKey, _, _, _, err := ssh.ParseAuthorizedKey(key)
	if err != nil {
		return nil, nil, err
	}

	return key, publicKey, nil
}

// WaitForSSH waits for SSH to become available on the given host.
// It polls port 22 until a connection can be established or the context is cancelled.
func WaitForSSH(ctx context.Context, host string) error {
	interval := 10 * time.Second
	addr := net.JoinHostPort(host, "22")

	for {
		conn, err := net.DialTimeout("tcp", addr, interval)
		if err == nil {
			conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
