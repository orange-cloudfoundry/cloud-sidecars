package config

import (
	"encoding/json"
	"fmt"
	"github.com/cloudfoundry-community/gautocloud/decoder"
)

type SidecarsConfig struct {
	Sidecars  []*SidecarConfig `yaml:"sidecars" json:"sidecars"`
	NoStarter bool             `yaml:"no_starter" json:"no_starter"`
	LogLevel  string           `json:"log_level" yaml:"log_level"`
	Dir       string           `json:"dir" yaml:"dir"`
	LogJson   bool             `json:"log_json" yaml:"log_json"`
	NoColor   bool             `json:"no_color" yaml:"no_color"`
	AppPort   int              `json:"app_port" yaml:"app_port"`
}

type SidecarConfig struct {
	Name                string            `yaml:"name" json:"name"`
	Executable          string            `yaml:"executable" json:"executable"`
	ArtifactURI         string            `yaml:"artifact_uri" json:"artifact_uri"`
	ArtifactType        string            `yaml:"artifact_type" json:"artifact_type"`
	ArtifactSha1        string            `yaml:"artifact_sha1" json:"artifact_sha1"`
	AfterDownload       string            `yaml:"after_download" json:"after_download"`
	Args                []string          `yaml:"args" json:"args"`
	Env                 map[string]string `yaml:"env" json:"env"`
	AppEnv              map[string]string `yaml:"app_env" json:"app_env"`
	ProfileD            string            `yaml:"profiled" json:"profiled"`
	WorkDir             string            `yaml:"work_dir" json:"work_dir"`
	NoLogPrefix         bool              `yaml:"no_log_prefix" json:"no_log_prefix"`
	IsRproxy            bool              `yaml:"is_rproxy" json:"is_rproxy"`
	NoInterruptWhenStop bool              `yaml:"no_interrupt_when_stop" json:"no_interrupt_when_stop"`
}

func (c SidecarConfig) Check() error {
	if c.Name == "" {
		return fmt.Errorf("You must provide a name to your sidecar")
	}
	if c.Executable == "" {
		return fmt.Errorf("You must provide an executable path to your sidecar")
	}
	return nil
}

func (c *SidecarConfig) UnmarshalCloud(data interface{}) error {
	type plain SidecarConfig
	err := decoder.Unmarshal(data.(map[string]interface{}), (*plain)(c))
	if err != nil {
		return err
	}
	return c.Check()
}

func (c *SidecarConfig) UnmarshalJSON(data []byte) error {
	type plain SidecarConfig
	err := json.Unmarshal(data, (*plain)(c))
	if err != nil {
		return err
	}
	return c.Check()
}

func (c *SidecarConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain SidecarConfig
	var err error
	if err = unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Check()
}
