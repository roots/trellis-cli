package trellis

import (
	"bufio"
	"crypto/rand"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/trellis/yamledit"
	"go.yaml.in/yaml/v3"
)

const fileHeader = "$ANSIBLE_VAULT;1.1;AES256"

func IsFileEncrypted(filepath string) (isEncrypted bool, err error) {
	file, err := os.Open(filepath)
	if err != nil {
		return false, err
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	line := scanner.Text()

	if strings.HasPrefix(line, fileHeader) {
		return true, nil
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}

	return false, nil
}

type StringGenerator interface {
	Generate() string
}

type RandomStringGenerator struct {
	Length int
}

func (rs *RandomStringGenerator) Generate() string {
	return generateRandomString(rs.Length)
}

// VaultPlaceholders are values treated as unset secrets: empty values and the
// examples shipped in Trellis' default vault files.
var VaultPlaceholders = []string{
	"",
	"generateme",
	"devpw",
	"stagingpw",
	"productionpw",
	"example_password",
	"example_dbpassword",
}

var wordpressSaltKeys = []string{
	"auth_key",
	"secure_auth_key",
	"logged_in_key",
	"nonce_key",
	"auth_salt",
	"secure_auth_salt",
	"logged_in_salt",
	"nonce_salt",
}

type VaultGenerateOptions struct {
	// Force regenerates secrets that are already set, not just placeholders.
	Force bool
	// SaltsOnly limits generation to the WordPress keys and salts.
	SaltsOnly bool
}

// GenerateVaultSecrets sets random values for the secrets of an environment's
// vault.yml document, adding any that are missing. Existing values are kept
// unless they are placeholders or opts.Force is set; Jinja templated values are
// never replaced. It returns the paths of the generated values.
func GenerateVaultSecrets(doc *yamledit.Document, env string, sites []string, randomString StringGenerator, opts VaultGenerateOptions) ([][]string, error) {
	assertAvailablePRNG()

	g := &vaultGenerator{doc: doc, randomString: randomString, force: opts.Force}

	if !opts.SaltsOnly {
		g.generate("vault_mysql_root_password")

		if env != "development" {
			g.generateUsers()
		}
	}

	for _, site := range sites {
		if !opts.SaltsOnly {
			// Only used by the wordpress-install role in development
			if env == "development" {
				g.generate("vault_wordpress_sites", site, "admin_password")
			}

			g.generate("vault_wordpress_sites", site, "env", "db_password")
		}

		for _, key := range wordpressSaltKeys {
			g.generate("vault_wordpress_sites", site, "env", key)
		}
	}

	return g.generated, g.err
}

// UpdateDefaultVaultFile renames the default site in a new project's vault.yml
// file (relative to the project) and generates all of its secrets.
func (t *Trellis) UpdateDefaultVaultFile(path string, name string, env string, header string) error {
	path = filepath.Join(t.Path, path)

	doc, err := yamledit.Load(path)
	if os.IsNotExist(err) {
		doc, err = yamledit.Parse(nil)
	}

	if err != nil {
		return err
	}

	if doc.Lookup("vault_wordpress_sites", DefaultSiteName) != nil {
		if err := doc.RenameKey(name, "vault_wordpress_sites", DefaultSiteName); err != nil {
			return err
		}
	}

	randomString := RandomStringGenerator{Length: 64}
	if _, err := GenerateVaultSecrets(doc, env, []string{name}, &randomString, VaultGenerateOptions{Force: true}); err != nil {
		return err
	}

	prependHeadComment(doc, header)

	return doc.Save(path)
}

type vaultGenerator struct {
	doc          *yamledit.Document
	randomString StringGenerator
	force        bool
	generated    [][]string
	err          error
}

func (g *vaultGenerator) generate(path ...string) {
	if g.err != nil {
		return
	}

	if node := g.doc.Lookup(path...); node != nil {
		if node.Kind != yaml.ScalarNode || strings.Contains(node.Value, "{{") {
			return
		}

		if !g.force && !slices.Contains(VaultPlaceholders, node.Value) {
			return
		}
	}

	if g.err = g.doc.Set(g.randomString.Generate(), path...); g.err == nil {
		g.generated = append(g.generated, path)
	}
}

func (g *vaultGenerator) generateUsers() {
	users := g.doc.Lookup("vault_users")

	if users == nil {
		g.err = g.doc.Set([]map[string]string{{"name": "{{ admin_user }}"}}, "vault_users")
		users = g.doc.Lookup("vault_users")
	}

	if g.err != nil || users.Kind != yaml.SequenceNode {
		return
	}

	for i := range users.Content {
		index := strconv.Itoa(i)
		g.generate("vault_users", index, "password")
		g.generate("vault_users", index, "salt")
	}
}

func (t *Trellis) GenerateVaultPassFile(path string) error {
	if !filepath.IsAbs(path) {
		path, _ = filepath.Rel(t.Path, filepath.Join(t.Path, path))
	}

	randomString := RandomStringGenerator{Length: 64}

	vaultPass := randomString.Generate()
	return os.WriteFile(path, []byte(vaultPass), 0600)
}

func assertAvailablePRNG() {
	buf := make([]byte, 1)

	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		log.Fatalf("Unable to generate random salt values. crypto/rand is unavailable: Read() failed with %#v", err)
	}
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)

	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}

	return b, nil
}

func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	bytes, _ := generateRandomBytes(n)

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return string(bytes)
}
