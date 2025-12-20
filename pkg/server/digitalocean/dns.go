package digitalocean

import (
	"context"
	"net/http"
	"strconv"

	"github.com/digitalocean/godo"
	"github.com/roots/trellis-cli/pkg/server/types"
)

const defaultTTL = 300

func (p *Provider) CreateZone(ctx context.Context, domain string) error {
	req := &godo.DomainCreateRequest{Name: domain}
	_, _, err := p.client.Domains.Create(ctx, req)
	return err
}

func (p *Provider) GetZone(ctx context.Context, domain string) (*types.Zone, bool, error) {
	d, resp, err := p.client.Domains.Get(ctx, domain)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}

	return &types.Zone{
		Name: d.Name,
		TTL:  d.TTL,
	}, true, nil
}

func (p *Provider) CreateRecord(ctx context.Context, domain string, record types.DNSRecord) (*types.DNSRecord, error) {
	ttl := record.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	req := &godo.DomainRecordEditRequest{
		Type: record.Type,
		Name: record.Name,
		Data: record.Value,
		TTL:  ttl,
	}

	r, _, err := p.client.Domains.CreateRecord(ctx, domain, req)
	if err != nil {
		return nil, err
	}

	return &types.DNSRecord{
		ID:    strconv.Itoa(r.ID),
		Type:  r.Type,
		Name:  r.Name,
		Value: r.Data,
		TTL:   r.TTL,
	}, nil
}

func (p *Provider) DeleteRecord(ctx context.Context, domain string, recordID string) error {
	id, err := strconv.Atoi(recordID)
	if err != nil {
		return err
	}

	_, err = p.client.Domains.DeleteRecord(ctx, domain, id)
	return err
}

func (p *Provider) ListRecords(ctx context.Context, domain string) ([]types.DNSRecord, error) {
	records, _, err := p.client.Domains.Records(ctx, domain, &godo.ListOptions{Page: 1, PerPage: 100})
	if err != nil {
		return nil, err
	}

	result := make([]types.DNSRecord, len(records))
	for i, r := range records {
		result[i] = types.DNSRecord{
			ID:    strconv.Itoa(r.ID),
			Type:  r.Type,
			Name:  r.Name,
			Value: r.Data,
			TTL:   r.TTL,
		}
	}

	return result, nil
}
