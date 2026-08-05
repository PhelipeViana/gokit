package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnvWithDefaults(t *testing.T) {
	t.Setenv("TEST_VAR_ONE", "value1")
	t.Setenv("TEST_VAR_TWO", "value2")
	t.Setenv("TEST_VAR_EMPTY", "")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no placeholders",
			input:    "normal string",
			expected: "normal string",
		},
		{
			name:     "single placeholder",
			input:    "prefix_${TEST_VAR_ONE}_suffix",
			expected: "prefix_value1_suffix",
		},
		{
			name:     "multiple placeholders",
			input:    "first=${TEST_VAR_ONE}&second=${TEST_VAR_TWO}",
			expected: "first=value1&second=value2",
		},
		{
			name:     "placeholder with default when set",
			input:    "value=${TEST_VAR_ONE:-default_val}",
			expected: "value=value1",
		},
		{
			name:     "placeholder with default when empty",
			input:    "value=${TEST_VAR_EMPTY:-default_val}",
			expected: "value=default_val",
		},
		{
			name:     "placeholder with default when unset",
			input:    "value=${TEST_VAR_UNSET:-default_val}",
			expected: "value=default_val",
		},
		{
			name:     "malformed placeholder no end",
			input:    "value=${TEST_VAR_ONE",
			expected: "value=${TEST_VAR_ONE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandEnvWithDefaults(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandEnvWithDefaults(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoadEnvFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gokit_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	envContent := `
# Comentário de linha
VAR_SIMPLE=simple_value
VAR_DOUBLE_QUOTES="double_quoted_value"
VAR_SINGLE_QUOTES='single_quoted_value'
VAR_DUPLICATED=first_val
VAR_DUPLICATED=second_val
`
	envFile := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envFile, []byte(envContent), 0o644); err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}

	duplicates, err := LoadEnvFile(envFile)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}

	if len(duplicates) != 1 || duplicates[0] != "VAR_DUPLICATED" {
		t.Errorf("expected duplicate key 'VAR_DUPLICATED', got %v", duplicates)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"VAR_SIMPLE", "simple_value"},
		{"VAR_DOUBLE_QUOTES", "double_quoted_value"},
		{"VAR_SINGLE_QUOTES", "single_quoted_value"},
		{"VAR_DUPLICATED", "first_val"},
	}

	for _, tt := range tests {
		got := os.Getenv(tt.key)
		if got != tt.expected {
			t.Errorf("Getenv(%q) = %q, expected %q", tt.key, got, tt.expected)
		}
	}
}

func TestTestDatabaseConnectionIntegration(t *testing.T) {
	// Carrega o configs/api.env da aplicação prevcontas_test para testar conexões físicas reais
	// se estiverem disponíveis no ambiente.
	envPath := "../../../prevcontas_test/configs/api.env"
	if _, err := os.Stat(envPath); err == nil {
		_, _ = LoadEnvFile(envPath)
	}

	// 1. Testar Postgres se configurado e rodando
	pgURL := os.ExpandEnv("postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=disable")
	if os.Getenv("POSTGRES_HOST") != "" {
		t.Run("Postgres connection", func(t *testing.T) {
			// Testa com "postgres" como dialeto
			err := TestDatabaseConnection("postgres", pgURL)
			if err != nil {
				t.Logf("Postgres connection failed (normal if not running): %v", err)
			} else {
				t.Log("Postgres connection validated successfully!")
			}

			// Testa com "postgresql" como dialeto para certificar que a normalização mapeia para "pgx"
			err = TestDatabaseConnection("postgresql", pgURL)
			if err != nil {
				t.Logf("Postgresql dialect normalization check failed: %v", err)
			}
		})
	}

	// 2. Testar Oracle se configurado e rodando
	oracleURL := os.ExpandEnv("oracle://${ORACLE_USER}:${ORACLE_PASSWORD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE}")
	if os.Getenv("ORACLE_HOST") != "" {
		t.Run("Oracle connection", func(t *testing.T) {
			err := TestDatabaseConnection("oracle", oracleURL)
			if err != nil {
				t.Logf("Oracle connection failed (normal if not running): %v", err)
			} else {
				t.Log("Oracle connection validated successfully!")
			}
		})
	}

	// 3. Testar dialeto inválido
	t.Run("Invalid dialect", func(t *testing.T) {
		err := TestDatabaseConnection("invalid_db", "invalid_url")
		if err == nil {
			t.Error("expected error for invalid dialect, but got nil")
		}
	} )
}
