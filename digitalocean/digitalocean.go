package digitalocean

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/hashicorp/cli"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/ssh"
)

const accessTokenEnvVar = "DIGITALOCEAN_ACCESS_TOKEN"

type Domain struct {
	Name   string
	Exists bool
}

type Host struct {
	Domain Domain
	Name   string
	Fqdn   string
	Error  error
	Record *godo.DomainRecord
}

func CheckSSH(host string, ctx context.Context) (err error) {
	interval := 10 * time.Second
	host = net.JoinHostPort(host, "22")

	for {
		_, err = net.DialTimeout("tcp", host, interval)

		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(interval):
		}
	}
}

func GetAccessToken(ui cli.Ui) (accessToken string, err error) {
	accessToken = os.Getenv(accessTokenEnvVar)

	if accessToken == "" {
		ui.Info(fmt.Sprintf("%s environment variable not set.", accessTokenEnvVar))
		accessToken, err = ui.Ask("Enter Access token:")

		if err != nil {
			return "", err
		}
	}

	return accessToken, nil
}

func LoadSSHKey(sshKeys []string) (keyPath string, contents []byte, publicKey ssh.PublicKey, err error) {
	for _, path := range sshKeys {
		keyPath = path
		contents, publicKey, err = loadPublicKey(path)

		if err == nil {
			break
		}
	}

	if publicKey == nil {
		return "", nil, nil, fmt.Errorf("No valid SSH public key found. Attempted paths: %s", strings.Join(sshKeys, ", "))
	}

	return keyPath, contents, publicKey, err
}

func loadPublicKey(path string) (contents []byte, publicKey ssh.PublicKey, err error) {
	path, err = homedir.Expand(path)
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	publicKey, _, _, _, err = ssh.ParseAuthorizedKey(key)
	if err != nil {
		return nil, nil, err
	}

	return key, publicKey, nil
}
