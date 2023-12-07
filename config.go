package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"time"
)

type TagConfig struct {
	Name      string `yaml:"name"`
	Desc      string `yaml:"desc"`
	Address   uint16 `yaml:"address"`
	Operation string `yaml:"operation"`
	Group     string `yaml:"group"`
}

type TelegramConfig struct {
	ApiToken string           `yaml:"apiToken"`
	Owners   map[int64]string `yaml:"owners"`
}

type Config struct {
	DeviceUrl   string         `yaml:"device-url"`
	DeviceId    uint8          `yaml:"device-id" default:"16"`
	Speed       uint           `yaml:"speed" default:"19200"`
	Timeout     time.Duration  `yaml:"timeout" default:"1s"`
	PollingTime time.Duration  `yaml:"polling-time" default:"1s"`
	ReadPeriod  time.Duration  `yaml:"read-period" default:"10ms"`
	Tags        []TagConfig    `yaml:"tags"`
	Telegram    TelegramConfig `yaml:"telegram"`
}

func NewConfig(configPath string) (config *Config, err error) {
	// Create configPath structure
	config = &Config{}

	// Open configPath file
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}

	// Init new YAML decode
	d := yaml.NewDecoder(file)

	// Start YAML decoding from file
	if err := d.Decode(&config); err != nil {
		return nil, err
	}

	return config, nil
}

// ValidateConfigPath just makes sure, that the path provided is a file,
// that can be read
func ValidateConfigPath(path string) error {
	s, err := os.Stat(path)
	if err != nil {
		return err
	}
	if s.IsDir() {
		return fmt.Errorf("'%s' is a directory, not a normal file", path)
	}
	return nil
}
