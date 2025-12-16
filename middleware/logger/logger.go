package logger

import (
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	Level   string `mapstructure:"level"`
	FilePath string `mapstructure:"file_path"`
}

func DefaultConfig() *Config {	
	return &Config{
		Level:   "info",
		FilePath: "logs/app.log",
	}
}

func ParseConfig(rawConfig map[string]interface{}) (*Config, error) {
	config := DefaultConfig()

	if rawConfig == nil {
		return config, nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
        Metadata:         nil,
        Result:           config,         
        TagName:          "mapstructure", 
        WeaklyTypedInput: true,
    })
	if err != nil {
		return nil, err
	}
	
	if err := decoder.Decode(rawConfig); err != nil {
		return nil, err
	}

	return config, nil
}