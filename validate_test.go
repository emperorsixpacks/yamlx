package yamlx

import (
	"testing"
)

type TestRequired struct {
	Name string `yaml:"name,required"`
}

type TestDefault struct {
	Port int `yaml:"port,omitempty,default=8080"`
}

type TestEnum struct {
	Env string `yaml:"env,enum=production|staging|development"`
}

type TestMinMax struct {
	Count int `yaml:"count,min=1,max=100"`
}

type TestMultiple struct {
	Name string `yaml:"name,required"`
	Port int    `yaml:"port,omitempty,default=8080"`
	Env  string `yaml:"env,enum=production|staging"`
	Count int   `yaml:"count,min=0,max=10"`
}

func TestRequiredValid(t *testing.T) {
	yml := `name: "hello"`
	var cfg TestRequired
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "hello" {
		t.Fatalf("expected 'hello', got %q", cfg.Name)
	}
}

func TestRequiredMissing(t *testing.T) {
	yml := `name: ""`
	var cfg TestRequired
	err := Unmarshal([]byte(yml), &cfg)
	if err == nil {
		t.Fatal("expected error for required field, got nil")
	}
	if !isConfigError(err) {
		t.Fatalf("expected configError, got %v", err)
	}
}

func TestDefaultApplied(t *testing.T) {
	yml := `{}`
	var cfg TestDefault
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected 8080, got %d", cfg.Port)
	}
}

func TestDefaultOverridden(t *testing.T) {
	yml := `port: 3000`
	var cfg TestDefault
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected 3000, got %d", cfg.Port)
	}
}

func TestEnumValid(t *testing.T) {
	yml := `env: production`
	var cfg TestEnum
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "production" {
		t.Fatalf("expected 'production', got %q", cfg.Env)
	}
}

func TestEnumInvalid(t *testing.T) {
	yml := `env: dev`
	var cfg TestEnum
	err := Unmarshal([]byte(yml), &cfg)
	if err == nil {
		t.Fatal("expected error for invalid enum value, got nil")
	}
	if !isConfigError(err) {
		t.Fatalf("expected configError, got %v", err)
	}
}

func TestMinMaxValid(t *testing.T) {
	yml := `count: 50`
	var cfg TestMinMax
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Count != 50 {
		t.Fatalf("expected 50, got %d", cfg.Count)
	}
}

func TestMinMaxBelowMin(t *testing.T) {
	yml := `count: 0`
	var cfg TestMinMax
	err := Unmarshal([]byte(yml), &cfg)
	if err == nil {
		t.Fatal("expected error for value below min, got nil")
	}
	if !isConfigError(err) {
		t.Fatalf("expected configError, got %v", err)
	}
}

func TestMinMaxAboveMax(t *testing.T) {
	yml := `count: 200`
	var cfg TestMinMax
	err := Unmarshal([]byte(yml), &cfg)
	if err == nil {
		t.Fatal("expected error for value above max, got nil")
	}
	if !isConfigError(err) {
		t.Fatalf("expected configError, got %v", err)
	}
}

func TestMultipleStructTags(t *testing.T) {
	yml := `
name: "app"
env: production
count: 5
`
	var cfg TestMultiple
	if err := Unmarshal([]byte(yml), &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "app" {
		t.Fatalf("expected 'app', got %q", cfg.Name)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected default 8080, got %d", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Fatalf("expected 'production', got %q", cfg.Env)
	}
	if cfg.Count != 5 {
		t.Fatalf("expected 5, got %d", cfg.Count)
	}
}

func TestSkipValidationTag(t *testing.T) {
	yml := `name: ""`
	var cfg TestRequired
	err := Unmarshal([]byte(yml), &cfg, SkipValidation())
	if err != nil {
		t.Fatalf("expected no error with SkipValidation, got: %v", err)
	}
}

func TestWithVarsTag(t *testing.T) {
	yml := `db: $db_host`
	var cfg struct {
		DB string `yaml:"db"`
	}
	if err := Unmarshal([]byte(yml), &cfg, WithVars(map[string]string{"db_host": "localhost"})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DB != "localhost" {
		t.Fatalf("expected 'localhost', got %q", cfg.DB)
	}
}

func isConfigError(err error) bool {
	_, ok := err.(configError)
	return ok
}
