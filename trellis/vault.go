package trellis

import (
	"crypto/rand"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
)

type StringGenerator interface {
	Generate() string
}

type RandomStringGenerator struct {
	Length int
}

func (rs *RandomStringGenerator) Generate() string {
	return generateRandomString(rs.Length)
}

type Vault struct {
	MysqlRootPassword string                        `yaml:"vault_mysql_root_password"`
	Users             []VaultUser                   `yaml:"vault_users,omitempty"`
	WordPressSites    map[string]VaultWordPressSite `yaml:"vault_wordpress_sites"`
}

type VaultUser struct {
	Name     string `yaml:"name"`
	Password string `yaml:"password"`
	Salt     string `yaml:"salt"`
}

type VaultWordPressSite struct {
	AdminPassword string                `yaml:"admin_password"`
	Env           VaultWordPressSiteEnv `yaml:"env"`
}

type VaultWordPressSiteEnv struct {
	DbPassword     string `yaml:"db_password"`
	AuthKey        string `yaml:"auth_key,omitempty"`
	SecureAuthKey  string `yaml:"secure_auth_key,omitempty"`
	LoggedInKey    string `yaml:"logged_in_key,omitempty"`
	NonceKey       string `yaml:"nonce_key,omitempty"`
	AuthSalt       string `yaml:"auth_salt,omitempty"`
	SecureAuthSalt string `yaml:"secure_auth_salt,omitempty"`
	LoggedInSalt   string `yaml:"logged_in_salt,omitempty"`
	NonceSalt      string `yaml:"nonce_salt,omitempty"`
}

func (t *Trellis) GenerateVaultConfig(name string, env string, randomString StringGenerator) *Vault {
	assertAvailablePRNG()
	var siteEnv VaultWordPressSiteEnv

	vault := Vault{MysqlRootPassword: randomString.Generate()}
	siteEnv = VaultWordPressSiteEnv{
		DbPassword:     randomString.Generate(),
		AuthKey:        randomString.Generate(),
		SecureAuthKey:  randomString.Generate(),
		LoggedInKey:    randomString.Generate(),
		NonceKey:       randomString.Generate(),
		AuthSalt:       randomString.Generate(),
		SecureAuthSalt: randomString.Generate(),
		LoggedInSalt:   randomString.Generate(),
		NonceSalt:      randomString.Generate(),
	}

	if env != "development" {
		user := VaultUser{
			Name:     "{{ admin_user }}",
			Password: randomString.Generate(),
			Salt:     randomString.Generate(),
		}

		vault.Users = []VaultUser{user}
	}

	vault.WordPressSites = make(map[string]VaultWordPressSite)
	vault.WordPressSites[name] = VaultWordPressSite{
		AdminPassword: randomString.Generate(),
		Env:           siteEnv,
	}

	return &vault
}

func (t *Trellis) GenerateVaultPassFile(path string) error {
	if !filepath.IsAbs(path) {
		path, _ = filepath.Rel(t.Path, filepath.Join(t.Path, path))
	}

	randomString := RandomStringGenerator{Length: 64}

	vaultPass := randomString.Generate()
	return ioutil.WriteFile(path, []byte(vaultPass), 0600)
}

func (t *Trellis) WriteVaultYaml(vault *Vault, path string) error {
	vaultYaml, err := yaml.Marshal(vault)

	if err != nil {
		log.Fatal(err)
	}

	return t.WriteYamlFile(path, vaultYaml)
}

func assertAvailablePRNG() {
	buf := make([]byte, 1)

	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		log.Fatal(fmt.Sprintf("Unable to generate random salt values. crypto/rand is unavailable: Read() failed with %#v", err))
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
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_[]{}<>~`+=,.;:/?|"

	bytes, _ := generateRandomBytes(n)

	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return string(bytes)
}
