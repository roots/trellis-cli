package digitalocean

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/user"
	"sort"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/roots/trellis-cli/dns"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"
)

const baseTag = "trellis"

var (
	ErrNotFound = errors.New("not found")
)

type Client struct {
	Client *godo.Client
}

func NewClient(accessToken string) *Client {
	token := &oauth2.Token{AccessToken: accessToken}
	t := oauth2.StaticTokenSource(token)
	oauthClient := oauth2.NewClient(context.Background(), t)
	client := godo.NewClient(oauthClient)

	return &Client{client}
}

func (do *Client) CreateDomain(name string) (domain *godo.Domain, err error) {
	createRequest := &godo.DomainCreateRequest{Name: name}
	ctx := context.TODO()
	domain, _, err = do.Client.Domains.Create(ctx, createRequest)

	if err != nil {
		return nil, err
	}

	return domain, nil
}

func (do *Client) CreateDomainRecord(domain string, hostName string, ip string) (domainRecord *godo.DomainRecord, err error) {
	createRequest := &godo.DomainRecordEditRequest{
		Type: "A",
		Name: hostName,
		Data: ip,
		TTL:  300,
	}

	ctx := context.TODO()

	domainRecord, _, err = do.Client.Domains.CreateRecord(ctx, domain, createRequest)

	if err != nil {
		return nil, err
	}

	return domainRecord, nil
}

func (do *Client) CreateDroplet(region string, size string, image string, publicKey ssh.PublicKey, name string, env string) (newDroplet *godo.Droplet, monitorUri string, err error) {
	createRequest := &godo.DropletCreateRequest{
		Name:   name,
		Region: region,
		Size:   size,
		Image: godo.DropletCreateImage{
			Slug: image,
		},
		SSHKeys: []godo.DropletCreateSSHKey{
			{Fingerprint: ssh.FingerprintLegacyMD5(publicKey)},
		},
		Tags: []string{baseTag, env},
	}

	ctx := context.TODO()

	newDroplet, response, err := do.Client.Droplets.Create(ctx, createRequest)
	if err != nil {
		return nil, "", err
	}

	monitorUri = response.Links.Actions[0].HREF

	return newDroplet, monitorUri, err
}

func (do *Client) CreateSSHKey(key string) error {
	var name string

	ctx := context.TODO()

	u, err := user.Current()
	if err != nil {
		name = "trellis-cli-ssh-key"
	} else {
		name = u.Username
	}

	createRequest := &godo.KeyCreateRequest{
		Name:      name,
		PublicKey: key,
	}

	_, _, err = do.Client.Keys.Create(ctx, createRequest)

	if err != nil {
		return fmt.Errorf("Could not create SSH key on DigitalOcean: %v", err)
	}

	return nil
}

func (do *Client) DeleteDomainRecord(record godo.DomainRecord, domain string) (err error) {
	ctx := context.TODO()

	_, err = do.Client.Domains.DeleteRecord(ctx, domain, record.ID)
	return err
}

func (do *Client) GetAvailableRegions() ([]godo.Region, error) {
	ctx := context.TODO()
	regions, _, err := do.Client.Regions.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	availableRegions := []godo.Region{}

	for _, region := range regions {
		if region.Available {
			availableRegions = append(availableRegions, region)
		}
	}

	sort.Slice(availableRegions, func(i, j int) bool {
		return availableRegions[i].Name < availableRegions[j].Name
	})

	return availableRegions, nil
}

func (do *Client) GetSizesByRegion(region *godo.Region) ([]godo.Size, error) {
	ctx := context.TODO()
	sizes, _, err := do.Client.Sizes.List(ctx, &godo.ListOptions{})
	if err != nil {
		return nil, err
	}

	availableSizes := []godo.Size{}

	for _, size := range sizes {
		if size.Available && sizeInRegion(region, size.Slug) && (strings.HasPrefix(size.Slug, "s-") || strings.HasPrefix(size.Slug, "c-")) {
			availableSizes = append(availableSizes, size)
		}
	}

	sort.Slice(availableSizes, func(i, j int) bool {
		return availableSizes[i].PriceMonthly < availableSizes[j].PriceMonthly
	})

	return availableSizes, nil
}

func sizeInRegion(region *godo.Region, sizeSlug string) bool {
	for _, size := range region.Sizes {
		if size == sizeSlug {
			return true
		}
	}

	return false
}

func (do *Client) GetSSHKey(publicKey ssh.PublicKey) (*godo.Response, error) {
	ctx := context.TODO()
	fingerprint := ssh.FingerprintLegacyMD5(publicKey)

	_, response, err := do.Client.Keys.GetByFingerprint(ctx, fingerprint)
	return response, err
}

func (do *Client) GetDroplet(droplet *godo.Droplet) (*godo.Droplet, string, error) {
	ctx := context.TODO()
	droplet, _, err := do.Client.Droplets.Get(ctx, droplet.ID)

	if err != nil {
		return nil, "", err
	}

	ip, err := droplet.PublicIPv4()

	if err != nil {
		return nil, "", err
	}

	return droplet, ip, err
}

func (do *Client) GetDroplets() (droplets []godo.Droplet, err error) {
	ctx := context.TODO()
	droplets, _, err = do.Client.Droplets.List(ctx, &godo.ListOptions{Page: 1, PerPage: 100})

	if err != nil {
		return nil, err
	}

	return droplets, err
}

func (do *Client) GetDropletByIP(ip string) (droplet *godo.Droplet, err error) {
	droplets, err := do.GetDroplets()
	if err != nil {
		return nil, err
	}

	for _, d := range droplets {
		dropletIP, err := d.PublicIPv4()
		if err != nil {
			continue
		}

		if ip == dropletIP {
			droplet = &d
			break
		}
	}

	return droplet, err
}

func (do *Client) GetHostRecords(hostsMap map[string][]dns.Host) []Host {
	allHosts := []Host{}

	for domain, hosts := range hostsMap {
		existingRecords, err := do.ListDomainRecords(domain)

		for _, host := range hosts {
			var hostRecord *godo.DomainRecord
			domainExists := true

			if errors.Is(err, ErrNotFound) {
				domainExists = false
			}

			for _, record := range existingRecords {
				if record.Name == host.Name {
					hostRecord = &record
					break
				}
			}

			host := Host{
				Name:   host.Name,
				Fqdn:   host.Fqdn,
				Error:  err,
				Record: hostRecord,
				Domain: Domain{Name: domain, Exists: domainExists},
			}

			allHosts = append(allHosts, host)
		}
	}

	return allHosts
}

func (do *Client) ListDomainRecords(domain string) (records []godo.DomainRecord, err error) {
	ctx := context.TODO()
	records, resp, err := do.Client.Domains.Records(ctx, domain, &godo.ListOptions{Page: 1, PerPage: 100})

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}

	if err != nil {
		return nil, err
	}

	return records, err
}
