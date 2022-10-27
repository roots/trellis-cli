package dns

import (
	"fmt"

	"github.com/weppos/publicsuffix-go/publicsuffix"
)

type Host struct {
	Domain string
	Name   string
	Fqdn   string
}

func ParseHost(hostName string) (host *Host, err error) {
	parsedDomain, err := publicsuffix.Parse(hostName)

	if err != nil {
		return nil, err
	}

	domain := fmt.Sprintf("%s.%s", parsedDomain.SLD, parsedDomain.TLD)

	name := parsedDomain.TRD

	if parsedDomain.TRD == "" {
		name = "@" // apex record name
	}

	return &Host{
		Domain: domain,
		Name:   name,
		Fqdn:   hostName,
	}, nil
}
