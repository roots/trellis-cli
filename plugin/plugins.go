package plugin

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type Plugins struct {
	Plugins []Plugin `yaml:"plugins"`
	filePath string
}

type Plugin struct {
	Command      string    `yaml:"command"`
	Bin          string    `yaml:"bin"`
	UpdateAt     time.Time `yaml:"updated_at"`
	SynopsisText string    `yaml:"synopsis_text"`
	HelpText     string    `yaml:"help_text"`
}

func NewPlugins(filePath string) (*Plugins, error) {
	// TODO: Fail if file exist but not readable.
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var plugins Plugins
	err = yaml.Unmarshal(content, &plugins)
	if err != nil {
		// TODO: Fail if file readable but not unmarshal-able.
		return nil, err
	}

	plugins.filePath = filePath

	return &plugins, nil
}

func (c *Plugins) Add(p Plugin) error {
	c.merge(p)
	return c.save()
}

func (c *Plugins) merge(p Plugin) {
	for index, plugin := range c.Plugins {
		if plugin.Command == p.Command {
			c.Plugins[index] = p
			return
		}
	}

	c.Plugins = append(c.Plugins, p)
}

func (c *Plugins) Remove(command string) error {
	for index, plugin := range c.Plugins {
		if plugin.Command == command {
			c.Plugins = append(c.Plugins[:index], c.Plugins[index+1:]...)
			break
		}
	}

	return c.save()
}

func (c *Plugins) save() error {
	content, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(c.filePath), 0771); err != nil {
		return err
	}

	return ioutil.WriteFile(c.filePath, content, 0600)
}
