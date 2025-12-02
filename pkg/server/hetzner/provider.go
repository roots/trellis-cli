package hetzner

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/roots/trellis-cli/pkg/server/types"
)

// Provider implements the types.Provider interface for Hetzner Cloud.
type Provider struct {
	client *hcloud.Client
}

// New creates a new Hetzner Cloud provider with the given API token.
func New(token string) *Provider {
	client := hcloud.NewClient(hcloud.WithToken(token))
	return &Provider{client: client}
}

func (p *Provider) Name() string        { return "hetzner" }
func (p *Provider) DisplayName() string { return "Hetzner Cloud" }

func (p *Provider) CreateServer(ctx context.Context, opts types.CreateServerOptions) (*types.Server, error) {
	labels := opts.Tags
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["trellis"] = "true"

	serverType, _, err := p.client.ServerType.GetByName(ctx, opts.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to get server type %s: %w", opts.Size, err)
	}
	if serverType == nil {
		return nil, fmt.Errorf("server type %s not found", opts.Size)
	}

	image, _, err := p.client.Image.GetByNameAndArchitecture(ctx, opts.Image, serverType.Architecture)
	if err != nil {
		return nil, fmt.Errorf("failed to get image %s: %w", opts.Image, err)
	}
	if image == nil {
		return nil, fmt.Errorf("image %s not found", opts.Image)
	}

	location, _, err := p.client.Location.GetByName(ctx, opts.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get location %s: %w", opts.Region, err)
	}
	if location == nil {
		return nil, fmt.Errorf("location %s not found", opts.Region)
	}

	sshKeys := make([]*hcloud.SSHKey, len(opts.SSHKeyIDs))
	for i, fp := range opts.SSHKeyIDs {
		key, _, err := p.client.SSHKey.GetByFingerprint(ctx, fp)
		if err != nil {
			return nil, fmt.Errorf("failed to get SSH key %s: %w", fp, err)
		}
		if key == nil {
			return nil, fmt.Errorf("SSH key with fingerprint %s not found", fp)
		}
		sshKeys[i] = key
	}

	result, _, err := p.client.Server.Create(ctx, hcloud.ServerCreateOpts{
		Name:       opts.Name,
		ServerType: serverType,
		Image:      image,
		Location:   location,
		SSHKeys:    sshKeys,
		Labels:     labels,
	})
	if err != nil {
		return nil, err
	}

	srv := p.hcloudToServer(result.Server)
	if result.Action != nil {
		srv.ID = fmt.Sprintf("%d:%d", result.Server.ID, result.Action.ID)
	}

	return srv, nil
}

func (p *Provider) GetServer(ctx context.Context, id string) (*types.Server, error) {
	intID, err := p.parseID(id)
	if err != nil {
		return nil, err
	}

	srv, _, err := p.client.Server.GetByID(ctx, intID)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, fmt.Errorf("server %d not found", intID)
	}

	return p.hcloudToServer(srv), nil
}

func (p *Provider) GetServers(ctx context.Context) ([]types.Server, error) {
	servers, err := p.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]types.Server, len(servers))
	for i, s := range servers {
		result[i] = *p.hcloudToServer(s)
	}

	return result, nil
}

func (p *Provider) WaitForServer(ctx context.Context, id string, timeout time.Duration) (*types.Server, error) {
	parts := splitID(id)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid server ID format for waiting: %s", id)
	}

	serverID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}

	actionID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	action, _, err := p.client.Action.GetByID(ctx, actionID)
	if err != nil {
		return nil, err
	}

	_, errCh := p.client.Action.WatchProgress(ctx, action)
	if err := <-errCh; err != nil {
		return nil, err
	}

	srv, _, err := p.client.Server.GetByID(ctx, serverID)
	if err != nil {
		return nil, err
	}

	return p.hcloudToServer(srv), nil
}

func (p *Provider) GetRegions(ctx context.Context) ([]types.Region, error) {
	locations, err := p.client.Location.All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]types.Region, len(locations))
	for i, loc := range locations {
		result[i] = types.Region{
			Slug:      loc.Name,
			Name:      loc.Description,
			Country:   loc.Country,
			Available: true,
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (p *Provider) GetSizes(ctx context.Context, region string) ([]types.Size, error) {
	serverTypes, err := p.client.ServerType.All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]types.Size, 0, len(serverTypes))
	for _, t := range serverTypes {
		available := true
		var priceMonthly, priceHourly float64

		if region != "" {
			available = false
			for _, price := range t.Pricings {
				if price.Location.Name == region {
					available = true
					priceMonthly, _ = strconv.ParseFloat(price.Monthly.Gross, 64)
					priceHourly, _ = strconv.ParseFloat(price.Hourly.Gross, 64)
					break
				}
			}
		} else if len(t.Pricings) > 0 {
			priceMonthly, _ = strconv.ParseFloat(t.Pricings[0].Monthly.Gross, 64)
			priceHourly, _ = strconv.ParseFloat(t.Pricings[0].Hourly.Gross, 64)
		}

		if available {
			result = append(result, types.Size{
				Slug:         t.Name,
				Name:         t.Description,
				VCPUs:        t.Cores,
				Memory:       int(t.Memory * 1024),
				Disk:         t.Disk,
				PriceMonthly: priceMonthly,
				PriceHourly:  priceHourly,
				Available:    true,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PriceMonthly < result[j].PriceMonthly
	})

	return result, nil
}

func (p *Provider) GetSSHKey(ctx context.Context, fingerprint string) (*types.SSHKey, error) {
	key, _, err := p.client.SSHKey.GetByFingerprint(ctx, fingerprint)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, nil
	}

	return &types.SSHKey{
		ID:          strconv.FormatInt(key.ID, 10),
		Name:        key.Name,
		Fingerprint: key.Fingerprint,
		PublicKey:   key.PublicKey,
	}, nil
}

func (p *Provider) CreateSSHKey(ctx context.Context, name string, publicKey string) (*types.SSHKey, error) {
	key, _, err := p.client.SSHKey.Create(ctx, hcloud.SSHKeyCreateOpts{
		Name:      name,
		PublicKey: publicKey,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create SSH key on Hetzner: %w", err)
	}

	return &types.SSHKey{
		ID:          strconv.FormatInt(key.ID, 10),
		Name:        key.Name,
		Fingerprint: key.Fingerprint,
		PublicKey:   key.PublicKey,
	}, nil
}

func (p *Provider) hcloudToServer(s *hcloud.Server) *types.Server {
	var ip, ipv6 string
	if s.PublicNet.IPv4.IP != nil {
		ip = s.PublicNet.IPv4.IP.String()
	}
	if s.PublicNet.IPv6.IP != nil {
		ipv6 = s.PublicNet.IPv6.IP.String()
	}

	var region, size string
	if s.Datacenter != nil && s.Datacenter.Location != nil {
		region = s.Datacenter.Location.Name
	}
	if s.ServerType != nil {
		size = s.ServerType.Name
	}

	return &types.Server{
		ID:           strconv.FormatInt(s.ID, 10),
		Name:         s.Name,
		Status:       mapStatus(s.Status),
		PublicIPv4:   ip,
		PublicIPv6:   ipv6,
		Region:       region,
		Size:         size,
		CreatedAt:    s.Created,
		DashboardURL: fmt.Sprintf("https://console.hetzner.cloud/servers/%d", s.ID),
	}
}

func (p *Provider) parseID(id string) (int64, error) {
	parts := splitID(id)
	return strconv.ParseInt(parts[0], 10, 64)
}

func splitID(id string) []string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

func mapStatus(s hcloud.ServerStatus) types.ServerStatus {
	switch s {
	case hcloud.ServerStatusInitializing:
		return types.ServerStatusPending
	case hcloud.ServerStatusStarting:
		return types.ServerStatusStarting
	case hcloud.ServerStatusRunning:
		return types.ServerStatusRunning
	case hcloud.ServerStatusStopping, hcloud.ServerStatusOff:
		return types.ServerStatusStopped
	default:
		return types.ServerStatusUnknown
	}
}
