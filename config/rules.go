package config

import (
	"fmt"
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

func ValidateRuleSet(rs *RuleSet) error {
	// Validate tiers
	for name, tier := range rs.Tiers {
		if tier.Capacity <= 0 {
			return fmt.Errorf("tier '%s': capacity must be positive", name)
		}
		if tier.RefillRate <= 0 {
			return fmt.Errorf("tier '%s': refill_rate must be positive", name)
		}
	}

	// Validate endpoints
	validRules := map[string]bool{
		"tiers+endpoints": true,
		"IP+endpoints":    true,
		"endpoint":        true,
	}

	for path, endpoint := range rs.Endpoints {
		if !validRules[endpoint.Rule] {
			return fmt.Errorf("endpoint '%s': unknown rule '%s'", path, endpoint.Rule)
		}
		if endpoint.Cost <= 0 {
			return fmt.Errorf("endpoint '%s': cost must be positive", path)
		}
		if endpoint.GlobalCapacity <= 0 {
			return fmt.Errorf("endpoint '%s': global_capacity must be positive", path)
		}
		if endpoint.GlobalRefillRate <= 0 {
			return fmt.Errorf("endpoint '%s': global_refill_rate must be positive", path)
		}
	}

	// Validate IPs
	if rs.IPs.Capacity <= 0 {
		return fmt.Errorf("ip config: capacity must be positive")
	}
	if rs.IPs.RefillRate <= 0 {
		return fmt.Errorf("ip config: refill_rate must be positive")
	}

	return nil
}
