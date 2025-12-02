package digitalocean

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/godo/util"
	"github.com/roots/trellis-cli/pkg/server/types"
	"golang.org/x/oauth2"
)

const baseTag = "trellis"

// Provider implements the types.Provider interface for DigitalOcean.
type Provider struct {
	client *godo.Client
}

// New creates a new DigitalOcean provider with the given API token.
func New(token string) *Provider {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	client := godo.NewClient(oauth2.NewClient(context.Background(), ts))
	return &Provider{client: client}
}

func (p *Provider) Name() string        { return "digitalocean" }
func (p *Provider) DisplayName() string { return "DigitalOcean" }

func (p *Provider) CreateServer(ctx context.Context, opts types.CreateServerOptions) (*types.Server, error) {
	tags := []string{baseTag}
	for k, v := range opts.Tags {
		tags = append(tags, fmt.Sprintf("%s:%s", k, v))
	}

	sshKeys := make([]godo.DropletCreateSSHKey, len(opts.SSHKeyIDs))
	for i, id := range opts.SSHKeyIDs {
		sshKeys[i] = godo.DropletCreateSSHKey{Fingerprint: id}
	}

	req := &godo.DropletCreateRequest{
		Name:    opts.Name,
		Region:  opts.Region,
		Size:    opts.Size,
		Image:   godo.DropletCreateImage{Slug: opts.Image},
		SSHKeys: sshKeys,
		Tags:    tags,
	}

	droplet, resp, err := p.client.Droplets.Create(ctx, req)
	if err != nil {
		return nil, err
	}

	srv := p.dropletToServer(droplet)

	if len(resp.Links.Actions) > 0 {
		srv.ID = fmt.Sprintf("%d:%s", droplet.ID, resp.Links.Actions[0].HREF)
	}

	return srv, nil
}

func (p *Provider) GetServer(ctx context.Context, id string) (*types.Server, error) {
	intID, err := p.parseID(id)
	if err != nil {
		return nil, err
	}

	droplet, _, err := p.client.Droplets.Get(ctx, intID)
	if err != nil {
		return nil, err
	}

	return p.dropletToServer(droplet), nil
}

func (p *Provider) GetServers(ctx context.Context) ([]types.Server, error) {
	droplets, _, err := p.client.Droplets.List(ctx, &godo.ListOptions{Page: 1, PerPage: 100})
	if err != nil {
		return nil, err
	}

	servers := make([]types.Server, len(droplets))
	for i, d := range droplets {
		servers[i] = *p.dropletToServer(&d)
	}

	return servers, nil
}

func (p *Provider) WaitForServer(ctx context.Context, id string, timeout time.Duration) (*types.Server, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid server ID format for waiting: %s", id)
	}

	dropletID, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}
	monitorURI := parts[1]

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err = util.WaitForActive(ctx, p.client, monitorURI)
	if err != nil {
		return nil, err
	}

	droplet, _, err := p.client.Droplets.Get(ctx, dropletID)
	if err != nil {
		return nil, err
	}

	return p.dropletToServer(droplet), nil
}

func (p *Provider) GetRegions(ctx context.Context) ([]types.Region, error) {
	regions, _, err := p.client.Regions.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]types.Region, 0, len(regions))
	for _, r := range regions {
		if r.Available {
			result = append(result, types.Region{
				Slug:      r.Slug,
				Name:      r.Name,
				Available: r.Available,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (p *Provider) GetSizes(ctx context.Context, region string) ([]types.Size, error) {
	sizes, _, err := p.client.Sizes.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]types.Size, 0)
	for _, s := range sizes {
		if !s.Available {
			continue
		}

		if !strings.HasPrefix(s.Slug, "s-") && !strings.HasPrefix(s.Slug, "c-") {
			continue
		}

		if region != "" && !sizeInRegion(s.Regions, region) {
			continue
		}

		result = append(result, types.Size{
			Slug:         s.Slug,
			VCPUs:        s.Vcpus,
			Memory:       s.Memory,
			Disk:         s.Disk,
			Transfer:     s.Transfer,
			PriceMonthly: s.PriceMonthly,
			PriceHourly:  s.PriceHourly,
			Available:    s.Available,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PriceMonthly < result[j].PriceMonthly
	})

	return result, nil
}

func sizeInRegion(regions []string, region string) bool {
	for _, r := range regions {
		if r == region {
			return true
		}
	}
	return false
}

func (p *Provider) GetSSHKey(ctx context.Context, fingerprint string) (*types.SSHKey, error) {
	key, resp, err := p.client.Keys.GetByFingerprint(ctx, fingerprint)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}

	return &types.SSHKey{
		ID:          strconv.Itoa(key.ID),
		Name:        key.Name,
		Fingerprint: key.Fingerprint,
		PublicKey:   key.PublicKey,
	}, nil
}

func (p *Provider) CreateSSHKey(ctx context.Context, name string, publicKey string) (*types.SSHKey, error) {
	req := &godo.KeyCreateRequest{
		Name:      name,
		PublicKey: publicKey,
	}

	key, _, err := p.client.Keys.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("could not create SSH key on DigitalOcean: %v", err)
	}

	return &types.SSHKey{
		ID:          strconv.Itoa(key.ID),
		Name:        key.Name,
		Fingerprint: key.Fingerprint,
		PublicKey:   key.PublicKey,
	}, nil
}

func (p *Provider) dropletToServer(d *godo.Droplet) *types.Server {
	ip, _ := d.PublicIPv4()
	ipv6, _ := d.PublicIPv6()

	var region, size string
	if d.Region != nil {
		region = d.Region.Slug
	}
	if d.Size != nil {
		size = d.Size.Slug
	}

	var createdAt time.Time
	if d.Created != "" {
		createdAt, _ = time.Parse(time.RFC3339, d.Created)
	}

	return &types.Server{
		ID:           strconv.Itoa(d.ID),
		Name:         d.Name,
		Status:       mapStatus(d.Status),
		PublicIPv4:   ip,
		PublicIPv6:   ipv6,
		Region:       region,
		Size:         size,
		CreatedAt:    createdAt,
		DashboardURL: fmt.Sprintf("https://cloud.digitalocean.com/droplets/%d", d.ID),
	}
}

func (p *Provider) parseID(id string) (int, error) {
	parts := strings.SplitN(id, ":", 2)
	return strconv.Atoi(parts[0])
}

func mapStatus(s string) types.ServerStatus {
	switch s {
	case "new":
		return types.ServerStatusPending
	case "active":
		return types.ServerStatusRunning
	case "off":
		return types.ServerStatusStopped
	default:
		return types.ServerStatusUnknown
	}
}
