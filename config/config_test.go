package config

import "testing"

func TestValidateRejectsBadPerClauseRules(t *testing.T) {
	cfg := defaultConfig()
	cfg.Layout.Clauses.When.Break = "sometimes"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.clauses.when.break")
	}
	cfg = defaultConfig()
	cfg.Layout.Clauses.Where.Align = "sideways"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.clauses.where.align")
	}
	cfg = defaultConfig()
	cfg.Layout.Content.WhenActions.Break = "bogus"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected an error for layout.content.when_actions.break")
	}
	if err := Validate(defaultConfig()); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestValidateRejectsBadPositions(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"comma_position":            func(c *Config) { c.Layout.Content.CommaPosition = "middle" },
		"logical_operator_position": func(c *Config) { c.Layout.Content.LogicalOperatorPosition = "middle" },
		"join_on_position":          func(c *Config) { c.Layout.Content.JoinOnPosition = "middle" },
	} {
		cfg := defaultConfig()
		mutate(cfg)
		if err := Validate(cfg); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}
