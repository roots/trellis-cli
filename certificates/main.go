package certificates

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/smallstep/certinfo"
	"github.com/smallstep/truststore"
)

func RootCertificatePath(configPath string) string {
	return filepath.Join(configPath, "root_certificates", "root_ca.crt")
}

func InstallFile(path string) error {
	opts := []truststore.Option{}
	opts = append(opts, truststore.WithFirefox(), truststore.WithJava(), truststore.WithDebug())

	if trustErr := truststore.InstallFile(path, opts...); trustErr != nil {
		switch err := trustErr.(type) {
		case *truststore.CmdError:
			return fmt.Errorf("failed to execute \"%s\" failed with: %v", strings.Join(err.Cmd().Args, " "), err.Error())
		default:
			return fmt.Errorf("failed to install %s: %v", path, err.Error())
		}
	}

	return nil
}

func UninstallFile(path string) error {
	opts := []truststore.Option{}
	opts = append(opts, truststore.WithFirefox(), truststore.WithJava())

	if trustErr := truststore.UninstallFile(path, opts...); trustErr != nil {
		switch err := trustErr.(type) {
		case *truststore.CmdError:
			return fmt.Errorf("failed to execute \"%s\" failed with: %v", strings.Join(err.Cmd().Args, " "), err.Error())
		default:
			return fmt.Errorf("failed to uninstall %s: %v", path, err.Error())
		}
	}

	return nil
}

func ShortText(cert *x509.Certificate) (info string, err error) {
	if s, err := certinfo.CertificateShortText(cert); err == nil {
		return s, nil
	}

	return "", fmt.Errorf("Error reading certificate: %v", err)
}

func FetchRootCertificate(path string, host string) (cert []byte, err error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	res, err := client.Get(fmt.Sprintf("https://%s:8443/roots.pem", host))
	if err != nil {
		return nil, fmt.Errorf("Could not fetch root certificate from server: %v", err)
	}

	body, err := io.ReadAll(res.Body)
	res.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("Could not read response from server: %v", err)
	}

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("Could not fetch root certificate from server: %d status code received", res.StatusCode)
	}

	return body, nil
}

func Trusted(cert *x509.Certificate) bool {
	chains, err := cert.Verify(x509.VerifyOptions{})
	return len(chains) > 0 && err == nil
}
