package digitalocean

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/ssh"
)

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
