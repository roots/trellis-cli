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

type Config struct {
	AllowDevelopmentDeploys bool              `yaml:"allow_development_deploys"`
	AskVaultPass            bool              `yaml:"ask_vault_pass"`
	CheckForUpdates         bool              `yaml:"check_for_updates"`
	LoadPlugins             bool              `yaml:"load_plugins"`
	Open                    map[string]string `yaml:"open"`
	VirtualenvIntegration   bool              `yaml:"virtualenv_integration"`
	VmManager               string            `yaml:"vm_manager"`
	VmHostsResolver         string            `yaml:"vm_hosts_resolver"`
}

var (
	ErrUnsupportedType = errors.New("Invalid env var config setting: value is an unsupported type.")
	ErrCouldNotParse   = errors.New("Invalid env var config setting: failed to parse value")
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
		return err
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
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as a boolean", ErrCouldNotParse, env, value)
					}

					structValue.SetBool(val)
				case reflect.Int:
					val, err := strconv.ParseInt(value, 10, 32)

					if err != nil {
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as an integer", ErrCouldNotParse, env, value)
					}

					structValue.SetInt(val)
				case reflect.Float32:
					val, err := strconv.ParseFloat(value, 32)
					if err != nil {
						return fmt.Errorf("%w '%s'\n'%s' can't be parsed as a float", ErrCouldNotParse, env, value)
					}

					structValue.SetFloat(val)
				default:
					return fmt.Errorf("%w\n%s setting of type %s is unsupported.", ErrUnsupportedType, env, field.Type.String())
				}
			}
		}
	}

	return nil
}
