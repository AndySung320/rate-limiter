package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type TierConfig struct {
	Capacity   int64 `yaml:"capacity"`
	RefillRate int64 `yaml:"refill_rate"`
}

type EndpointConfig struct {
	Rule             string `yaml:"rule"`
	Cost             int64  `yaml:"cost"`
	GlobalCapacity   int64  `yaml:"global_capacity"`
	GlobalRefillRate int64  `yaml:"global_refill_rate"`
}

type IPConfig struct {
	Capacity   int64 `yaml:"capacity"`
	RefillRate int64 `yaml:"refill_rate"`
}

type RuleSet struct {
	Tiers     map[string]TierConfig     `yaml:"tiers"`
	Endpoints map[string]EndpointConfig `yaml:"endpoints"`
	IPs       IPConfig                  `yaml:"ips"`
}

func LoadRuleSet(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ruleSet RuleSet
	if err := yaml.Unmarshal(data, &ruleSet); err != nil {
		return nil, err
	}

	return &ruleSet, nil
}
