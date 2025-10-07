// config/rules_test.go
package config

import (
	"os"
	"testing"
)

func TestLoadRuleSet_ValidConfig(t *testing.T) {
	ruleSet, err := LoadRuleSet("testdata/valid_config.yaml")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Test tiers loaded correctly
	if len(ruleSet.Tiers) != 2 {
		t.Errorf("expected 2 tiers, got %d", len(ruleSet.Tiers))
	}

	freeTier, exists := ruleSet.Tiers["free"]
	if !exists {
		t.Error("expected 'free' tier to exist")
	}
	if freeTier.Capacity != 100 {
		t.Errorf("expected free tier capacity 100, got %d", freeTier.Capacity)
	}
	if freeTier.RefillRate != 10 {
		t.Errorf("expected free tier refill rate 10, got %d", freeTier.RefillRate)
	}

	// Test endpoints loaded correctly
	if len(ruleSet.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(ruleSet.Endpoints))
	}

	endpoint, exists := ruleSet.Endpoints["/api/test"]
	if !exists {
		t.Error("expected '/api/test' endpoint to exist")
	}
	if endpoint.Rule != "tiers+endpoints" {
		t.Errorf("expected rule 'tiers+endpoints', got '%s'", endpoint.Rule)
	}
	if endpoint.Cost != 10 {
		t.Errorf("expected cost 10, got %d", endpoint.Cost)
	}

	// Test IPs loaded correctly
	if ruleSet.IPs.Capacity != 500 {
		t.Errorf("expected IP capacity 500, got %d", ruleSet.IPs.Capacity)
	}
}

func TestLoadRuleSet_FileNotFound(t *testing.T) {
	_, err := LoadRuleSet("nonexistent.yaml")

	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadRuleSet_InvalidSyntax(t *testing.T) {
	ruleSet, err := LoadRuleSet("testdata/invalid_syntax.yaml")

	if err == nil {
		t.Error("expected error for invalid YAML syntax")
	}
	if ruleSet != nil {
		t.Error("expected nil ruleset on error")
	}
}

func TestLoadRuleSet_EmptyFile(t *testing.T) {
	// Create temporary empty file
	tmpFile, _ := os.CreateTemp("", "empty_*.yaml")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	ruleSet, err := LoadRuleSet(tmpFile.Name())

	if err != nil {
		t.Errorf("empty file should parse without error, got: %v", err)
	}
	if len(ruleSet.Tiers) != 0 {
		t.Error("expected empty tiers map")
	}
	if len(ruleSet.Endpoints) != 0 {
		t.Error("expected empty endpoints map")
	}
}

func TestLoadRuleSet_MalformedYAML(t *testing.T) {
	// Create temporary malformed YAML
	tmpFile, _ := os.CreateTemp("", "malformed_*.yaml")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("tiers:\n  free:\n    capacity: [this is not valid\n")
	tmpFile.Close()

	_, err := LoadRuleSet(tmpFile.Name())

	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

func TestValidateRuleSet(t *testing.T) {
	tests := []struct {
		name      string
		ruleSet   *RuleSet
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid ruleset",
			ruleSet: &RuleSet{
				Tiers: map[string]TierConfig{
					"free": {Capacity: 100, RefillRate: 10},
				},
				Endpoints: map[string]EndpointConfig{
					"/api/test": {
						Rule:             "tiers+endpoints",
						Cost:             10,
						GlobalCapacity:   1000,
						GlobalRefillRate: 100,
					},
				},
				IPs: IPConfig{Capacity: 500, RefillRate: 50},
			},
			wantError: false,
		},
		{
			name: "negative capacity",
			ruleSet: &RuleSet{
				Tiers: map[string]TierConfig{
					"free": {Capacity: -100, RefillRate: 10},
				},
			},
			wantError: true,
			errorMsg:  "capacity must be positive",
		},
		{
			name: "zero refill rate",
			ruleSet: &RuleSet{
				Tiers: map[string]TierConfig{
					"free": {Capacity: 100, RefillRate: 0},
				},
			},
			wantError: true,
			errorMsg:  "refill_rate must be positive",
		},
		{
			name: "invalid rule type",
			ruleSet: &RuleSet{
				Endpoints: map[string]EndpointConfig{
					"/api/test": {
						Rule: "invalid_rule",
						Cost: 10,
					},
				},
			},
			wantError: true,
			errorMsg:  "unknown rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuleSet(tt.ruleSet)

			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr))
}
