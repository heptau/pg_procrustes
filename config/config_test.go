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
