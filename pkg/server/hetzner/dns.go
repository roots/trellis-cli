package hetzner

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/roots/trellis-cli/pkg/server/types"
)

const defaultTTL = 300

func (p *Provider) CreateZone(ctx context.Context, domain string) error {
	_, _, err := p.client.Zone.Create(ctx, hcloud.ZoneCreateOpts{
		Name: domain,
	})
	return err
}

func (p *Provider) GetZone(ctx context.Context, domain string) (*types.Zone, bool, error) {
	zone, _, err := p.client.Zone.GetByName(ctx, domain)
	if err != nil {
		return nil, false, err
	}
	if zone == nil {
		return nil, false, nil
	}

	return &types.Zone{
		ID:   strconv.FormatInt(zone.ID, 10),
		Name: zone.Name,
		TTL:  zone.TTL,
	}, true, nil
}

func (p *Provider) CreateRecord(ctx context.Context, domain string, record types.DNSRecord) (*types.DNSRecord, error) {
	zone, _, err := p.client.Zone.GetByName(ctx, domain)
	if err != nil {
		return nil, err
	}
	if zone == nil {
		return nil, fmt.Errorf("zone %s not found", domain)
	}

	ttl := record.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	result, _, err := p.client.Zone.CreateRRSet(ctx, zone, hcloud.ZoneRRSetCreateOpts{
		Name: record.Name,
		Type: hcloud.ZoneRRSetType(record.Type),
		TTL:  &ttl,
		Records: []hcloud.ZoneRRSetRecord{
			{Value: record.Value},
		},
	})
	if err != nil {
		return nil, err
	}

	rrset := result.RRSet
	var rrsetTTL int
	if rrset.TTL != nil {
		rrsetTTL = *rrset.TTL
	}

	return &types.DNSRecord{
		ID:    rrset.ID,
		Type:  string(rrset.Type),
		Name:  rrset.Name,
		Value: record.Value,
		TTL:   rrsetTTL,
	}, nil
}

func (p *Provider) DeleteRecord(ctx context.Context, domain string, recordID string) error {
	zone, _, err := p.client.Zone.GetByName(ctx, domain)
	if err != nil {
		return err
	}
	if zone == nil {
		return fmt.Errorf("zone %s not found", domain)
	}

	rrset, _, err := p.client.Zone.GetRRSetByID(ctx, zone, recordID)
	if err != nil {
		return err
	}
	if rrset == nil {
		return fmt.Errorf("record %s not found", recordID)
	}

	_, _, err = p.client.Zone.DeleteRRSet(ctx, rrset)
	return err
}

func (p *Provider) ListRecords(ctx context.Context, domain string) ([]types.DNSRecord, error) {
	zone, _, err := p.client.Zone.GetByName(ctx, domain)
	if err != nil {
		return nil, err
	}
	if zone == nil {
		return []types.DNSRecord{}, nil
	}

	rrsets, err := p.client.Zone.AllRRSets(ctx, zone)
	if err != nil {
		return nil, err
	}

	var result []types.DNSRecord
	for _, rrset := range rrsets {
		var ttl int
		if rrset.TTL != nil {
			ttl = *rrset.TTL
		}
		for _, record := range rrset.Records {
			result = append(result, types.DNSRecord{
				ID:    rrset.ID,
				Type:  string(rrset.Type),
				Name:  rrset.Name,
				Value: record.Value,
				TTL:   ttl,
			})
		}
	}

	return result, nil
}
