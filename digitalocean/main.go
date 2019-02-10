package digitalocean

import (
	"context"
	"os/user"
	"sort"
	"strings"

	"github.com/digitalocean/godo"
	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"
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

func (do *Client) CreateDroplet(region string, size string, publicKey ssh.PublicKey, name string, env string) (newDroplet *godo.Droplet, monitorUri string, err error) {
	createRequest := &godo.DropletCreateRequest{
		Name:   name,
		Region: region,
		Size:   size,
		Image: godo.DropletCreateImage{
			Slug: "ubuntu-18-04-x64",
		},
		SSHKeys: []godo.DropletCreateSSHKey{
			{Fingerprint: ssh.FingerprintLegacyMD5(publicKey)},
		},
		Tags: []string{"trellis", env},
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
	}

	name = u.Username

	createRequest := &godo.KeyCreateRequest{
		Name:      name,
		PublicKey: key,
	}

	_, _, err = do.Client.Keys.Create(ctx, createRequest)

	if err != nil {
		return err
	}

	return nil
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
