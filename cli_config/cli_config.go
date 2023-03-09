package cli_config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type VmImage struct {
	Location string `yaml:"location"`
	Arch     string `yaml:"arch"`
}

type VmConfig struct {
	Manager       string    `yaml:"manager"`
	HostsResolver string    `yaml:"hosts_resolver"`
	Images        []VmImage `yaml:"images"`
	Ubuntu        string    `yaml:"ubuntu"`
}

type Config struct {
	AllowDevelopmentDeploys bool              `yaml:"allow_development_deploys"`
	AskVaultPass            bool              `yaml:"ask_vault_pass"`
	DatabaseApp             string            `yaml:"database_app"`
	CheckForUpdates         bool              `yaml:"check_for_updates"`
	LoadPlugins             bool              `yaml:"load_plugins"`
	Open                    map[string]string `yaml:"open"`
	VirtualenvIntegration   bool              `yaml:"virtualenv_integration"`
	Vm                      VmConfig          `yaml:"vm"`
}

var (
	UnsupportedTypeErr = errors.New("Invalid env var config setting: value is an unsupported type.")
	CouldNotParseErr   = errors.New("Invalid env var config setting: failed to parse value")
	InvalidConfigErr   = errors.New("Invalid config file")
)

func NewConfig(defaultConfig Config) Config {
	return defaultConfig
}

func (c *Config) LoadFile(path string) error {
	configYaml, err := os.ReadFile(path)

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := yaml.Unmarshal(configYaml, &c); err != nil {
		return fmt.Errorf("%w: %s", InvalidConfigErr, err)
	}

	if c.Vm.Manager != "lima" && c.Vm.Manager != "auto" && c.Vm.Manager != "mock" {
		return fmt.Errorf("%w: unsupported value for `vm.manager`. Must be one of: auto, lima", InvalidConfigErr)
	}

	if c.Vm.Ubuntu != "18.04" && c.Vm.Ubuntu != "20.04" && c.Vm.Ubuntu != "22.04" {
		return fmt.Errorf("%w: unsupported value for `vm.ubuntu`. Must be one of: 18.04, 20.04, 22.04", InvalidConfigErr)
	}

	if c.Vm.HostsResolver != "hosts_file" {
		return fmt.Errorf("%w: unsupported value for `vm.hosts_resolver`. Must be one of: hosts_file", InvalidConfigErr)
	}

	if c.DatabaseApp != "" && c.DatabaseApp != "tableplus" && c.DatabaseApp != "sequel-ace" {
		return fmt.Errorf("%w: unsupported value for `database_app`. Must be one of: tableplus, sequel-ace", InvalidConfigErr)
	}

	return nil
}

func (c *Config) LoadEnv(prefix string) error {
	structType := reflect.ValueOf(c).Elem()
	fields := reflect.VisibleFields(structType.Type())

	for _, env := range os.Environ() {
		parts := strings.Split(env, "=")
		originalKey := parts[0]
		value := parts[1]

		key := strings.TrimPrefix(originalKey, prefix)

		if originalKey == key {
			// key is unchanged and didn't start with prefix
			continue
		}

		for _, field := range fields {
			if strings.ToLower(key) == field.Tag.Get("yaml") {
				structValue := structType.FieldByName(field.Name)

				if !structValue.CanSet() {
					continue
				}

				switch field.Type.Kind() {
				case reflect.Bool:
					val, err := strconv.ParseBool(value)

					if err != nil {
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as a boolean", CouldNotParseErr, env, value)
					}

					structValue.SetBool(val)
				case reflect.Int:
					val, err := strconv.ParseInt(value, 10, 32)

					if err != nil {
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as an integer", CouldNotParseErr, env, value)
					}

					structValue.SetInt(val)
				case reflect.Float32:
					val, err := strconv.ParseFloat(value, 32)
					if err != nil {
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as a float", CouldNotParseErr, env, value)
					}

					structValue.SetFloat(val)
				default:
					return fmt.Errorf("%w\n%s setting of type %s is unsupported.", UnsupportedTypeErr, env, field.Type.String())
				}
			}
		}
	}

	return nil
}
