package config

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Drivers de banco de dados
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"
)

// Scaffold padrão de gokit.json
const DefaultScaffoldJSON = `{
  "environment": {
    "mapper_env": "./.env",
    "ambient": "APP_ENV",
    "client": "DB_CLIENT"
  },
  "connections": {
    "client_postgres": {
      "dialect": "postgres",
      "host": "${POSTGRES_HOST}",
      "port": "${POSTGRES_PORT}",
      "user": "${POSTGRES_USER}",
      "password": "${POSTGRES_PASSWORD}",
      "database": "${POSTGRES_DB}",
      "sslmode": "${POSTGRES_SSLMODE:-disable}",
      "schema": "${POSTGRES_SCHEMA:-public}"
    },
    "client_oracle": {
      "dialect": "oracle",
      "host": "${ORACLE_HOST}",
      "port": "${ORACLE_PORT}",
      "user": "${ORACLE_USER}",
      "password": "${ORACLE_PASSWORD}",
      "service": "${ORACLE_SERVICE}",
      "schema": "${ORACLE_SCHEMA:-AGENDAGOKIT}"
    },
    "client_mysql": {
      "dialect": "mysql",
      "host": "${MYSQL_HOST}",
      "port": "${MYSQL_PORT}",
      "user": "${MYSQL_USER}",
      "password": "${MYSQL_PASSWORD}",
      "database": "${MYSQL_DATABASE}"
    },
    "client_sqlserver": {
      "dialect": "sqlserver",
      "host": "${MSSQL_HOST}",
      "port": "${MSSQL_PORT}",
      "user": "${MSSQL_USER}",
      "password": "${MSSQL_SA_PASSWORD}",
      "database": "${MSSQL_DATABASE}",
      "schema": "${MSSQL_SCHEMA:-dbo}"
    }
  },
  "output": {
    "settings": "internal/gokit/",
    "orm": "internal/gokit/orm",
    "migrate": "internal/gokit/migrate",
    "factory": "internal/gokit/factory",
    "seed": "internal/gokit/seed",
    "docs": "internal/gokit/docs"
  },
  "migrate": {
    "table": "migrations_gokit"
  },
  "seed": {
    "table": "seeders_gokit"
  },
  "factory": {
    "expressions": {
      "mappers": {
        "tabela.campo": "gokit.FakeMetodo(index, 0, 1)"
      }
    }
  },
  "notifications": {
    "slack": {
      "enabled": false,
      "webhook_url": "${SLACK_WEBHOOK_URL}"
    }
  }
}`

// Definições de estruturas para carregar o gokit.json
type Config struct {
	Environment   EnvConfig             `json:"environment"`
	Connections   map[string]ConnConfig `json:"connections"`
	Output        OutputConfig          `json:"output"`
	Migrate       MigrateConfig         `json:"migrate"`
	Seed          SeedConfig            `json:"seed"`
	Factory       FactoryConfig         `json:"factory"`
	Notifications NotificationsConfig   `json:"notifications"`
}

type EnvConfig struct {
	MapperEnv string `json:"mapper_env"`
	Ambient   string `json:"ambient"`
	Client    string `json:"client"`
}

type ConnConfig struct {
	Dialect  string `json:"dialect"`
	Host     string `json:"host,omitempty"`
	Port     string `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	Service  string `json:"service,omitempty"`
	SSLMode  string `json:"sslmode,omitempty"`
	Schema   string `json:"schema,omitempty"`
}

func (c ConnConfig) BuildURL() string {
	dialect := strings.ToLower(strings.TrimSpace(c.Dialect))
	switch dialect {
	case "postgres", "postgresql":
		ssl := c.SSLMode
		if ssl == "" {
			ssl = "disable"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			c.User, c.Password, c.Host, c.Port, c.Database, ssl)

	case "oracle":
		return fmt.Sprintf("oracle://%s:%s@%s:%s/%s",
			c.User, c.Password, c.Host, c.Port, c.Service)

	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
			c.User, c.Password, c.Host, c.Port, c.Database)

	case "sqlserver":
		return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s&encrypt=disable",
			c.User, c.Password, c.Host, c.Port, c.Database)

	default:
		return ""
	}
}

type OutputConfig struct {
	Settings string `json:"settings"`
	ORM      string `json:"orm"`
	Migrate  string `json:"migrate"`
	Factory  string `json:"factory"`
	Seed     string `json:"seed"`
	Docs     string `json:"docs"`
}

type MigrateConfig struct {
	Table string `json:"table"`
}

type SeedConfig struct {
	Table string `json:"table"`
}

type FactoryConfig struct {
	Expressions FactoryExpressions `json:"expressions"`
}

type FactoryExpressions struct {
	Mappers map[string]string `json:"mappers"`
}

type NotificationsConfig struct {
	Slack SlackConfig `json:"slack"`
}

type SlackConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

type ConfigState struct {
	ScaffoldCreated bool
	ConfigPath      string
	ConfigFileError error
	Config          *Config
	EnvWarnings     []string
	ActiveClient    string
	ActiveDialect   string
	ActiveURL       string
	ConnError       error
	ConnSuccess     bool
	ActiveEnv       string
}

// LoadEnvFile lê o arquivo .env, injeta as variáveis no sistema e retorna chaves duplicadas
func LoadEnvFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	seenKeys := make(map[string]bool)
	var duplicates []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if seenKeys[key] {
			alreadyWarned := false
			for _, d := range duplicates {
				if d == key {
					alreadyWarned = true
					break
				}
			}
			if !alreadyWarned {
				duplicates = append(duplicates, key)
			}
		}
		seenKeys[key] = true

		// Remove aspas
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return duplicates, scanner.Err()
}

// ExpandEnvWithDefaults preenche ${VAR} ou ${VAR:-default}
func ExpandEnvWithDefaults(str string) string {
	for {
		start := strings.Index(str, "${")
		if start == -1 {
			break
		}
		end := strings.Index(str[start:], "}")
		if end == -1 {
			break
		}
		endIdx := start + end

		inner := str[start+2 : endIdx]
		varName := inner
		defaultVal := ""

		if strings.Contains(inner, ":-") {
			parts := strings.SplitN(inner, ":-", 2)
			varName = parts[0]
			defaultVal = parts[1]
		}

		val := os.Getenv(varName)
		if val == "" {
			val = defaultVal
		}

		str = str[:start] + val + str[endIdx+1:]
	}
	return str
}

// EnsureConfigExistsAndLoad lê o gokit.json, criando a pasta/arquivo se não existirem
func EnsureConfigExistsAndLoad() (*Config, string, []string, error) {
	configPath := filepath.Join("internal", "gokit", "gokit.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Join("internal", "gokit"), 0o755)
		if err != nil {
			return nil, configPath, nil, fmt.Errorf("falha ao criar pasta internal/gokit: %v", err)
		}
		err = os.WriteFile(configPath, []byte(DefaultScaffoldJSON), 0o644)
		if err != nil {
			return nil, configPath, nil, fmt.Errorf("falha ao gerar gokit.json: %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, nil, fmt.Errorf("não foi possível ler o arquivo: %v", err)
	}

	var rawConfig Config
	err = json.Unmarshal(data, &rawConfig)
	if err != nil {
		return nil, configPath, nil, fmt.Errorf("sintaxe JSON inválida: %v", err)
	}

	// Carrega arquivo .env se mapeado
	envPath := rawConfig.Environment.MapperEnv
	var envWarnings []string
	if envPath != "" {
		envWarnings, _ = LoadEnvFile(envPath)
	}

	// Substitui variáveis de ambiente nas conexões
	interpolatedConfig := rawConfig
	interpolatedConfig.Connections = make(map[string]ConnConfig)
	for name, conn := range rawConfig.Connections {
		interpolatedConfig.Connections[name] = ConnConfig{
			Dialect:  conn.Dialect,
			Host:     ExpandEnvWithDefaults(conn.Host),
			Port:     ExpandEnvWithDefaults(conn.Port),
			User:     ExpandEnvWithDefaults(conn.User),
			Password: ExpandEnvWithDefaults(conn.Password),
			Database: ExpandEnvWithDefaults(conn.Database),
			Service:  ExpandEnvWithDefaults(conn.Service),
			SSLMode:  ExpandEnvWithDefaults(conn.SSLMode),
			Schema:   ExpandEnvWithDefaults(conn.Schema),
		}
	}
	interpolatedConfig.Notifications.Slack.WebhookURL = ExpandEnvWithDefaults(rawConfig.Notifications.Slack.WebhookURL)

	return &interpolatedConfig, configPath, envWarnings, nil
}

// TestDatabaseConnection abre conexão e faz ping
func TestDatabaseConnection(dialect, connURL string) error {
	driverName := strings.ToLower(strings.TrimSpace(dialect))
	if driverName == "postgres" || driverName == "postgresql" {
		driverName = "pgx"
	}

	db, err := sql.Open(driverName, connURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return db.PingContext(ctx)
}

// MaskPassword oculta senhas em conexões SQL para log mais seguro
func MaskPassword(dialect, url string) string {
	if dialect == "mysql" {
		parts := strings.SplitN(url, "@", 2)
		if len(parts) == 2 {
			subParts := strings.SplitN(parts[0], ":", 2)
			if len(subParts) == 2 {
				return subParts[0] + ":***@" + parts[1]
			}
		}
		return url
	}

	if strings.Contains(url, "://") {
		parts := strings.SplitN(url, "://", 2)
		scheme := parts[0]
		rest := parts[1]

		subParts := strings.SplitN(rest, "@", 2)
		if len(subParts) == 2 {
			credentials := subParts[0]
			hostPart := subParts[1]

			credParts := strings.SplitN(credentials, ":", 2)
			if len(credParts) == 2 {
				return scheme + "://" + credParts[0] + ":***@" + hostPart
			}
		}
	}

	return url
}

// RunConfigChecks executa o fluxo completo do módulo 1) Configuração
func RunConfigChecks() ConfigState {
	var state ConfigState

	configPath := filepath.Join("internal", "gokit", "gokit.json")
	_, statErr := os.Stat(configPath)
	state.ScaffoldCreated = os.IsNotExist(statErr)

	cfg, path, warnings, err := EnsureConfigExistsAndLoad()
	state.ConfigPath = path
	state.EnvWarnings = warnings
	if err != nil {
		state.ConfigFileError = err
		return state
	}
	state.Config = cfg

	// Determina o ambiente ativo
	envKey := cfg.Environment.Ambient
	if envKey == "" {
		envKey = "APP_ENV"
	}
	activeEnv := os.Getenv(envKey)
	if activeEnv == "" {
		for _, altKey := range []string{"APP_ENV", "ENV", "ENVIRONMENT"} {
			if val := os.Getenv(altKey); val != "" {
				activeEnv = val
				break
			}
		}
		if activeEnv == "" {
			activeEnv = "development"
		}
	}
	state.ActiveEnv = activeEnv

	// Determina o cliente ativo
	clientKey := cfg.Environment.Client
	if clientKey == "" {
		clientKey = "DB_CLIENT"
	}
	activeClient := os.Getenv(clientKey)
	if activeClient == "" {
		// Tenta buscar por outras variáveis comuns no .env caso a chave definida esteja vazia
		for _, altKey := range []string{"DB_DIALECT", "DB_CONNECTION", "DB_CLIENT"} {
			if val := os.Getenv(altKey); val != "" {
				activeClient = val
				break
			}
		}
		if activeClient == "" {
			activeClient = "postgres" // Padrão
		}
	}
	state.ActiveClient = activeClient

	// Busca a conexão com busca inteligente e tolerante a falhas
	var conn ConnConfig
	var found bool
	var matchedKey string

	// 1. Tenta correspondência exata
	if c, ok := cfg.Connections[activeClient]; ok {
		conn = c
		found = true
		matchedKey = activeClient
	}

	// 2. Tenta correspondência tolerante a maiúsculas/minúsculas e sem prefixo "client_"
	if !found {
		normActive := strings.ToLower(strings.TrimPrefix(activeClient, "client_"))
		for name, c := range cfg.Connections {
			normName := strings.ToLower(strings.TrimPrefix(name, "client_"))
			if normName == normActive {
				conn = c
				found = true
				matchedKey = name
				break
			}
		}
	}

	// 3. Tenta correspondência pelo campo "dialect"
	if !found {
		for name, c := range cfg.Connections {
			if strings.ToLower(c.Dialect) == strings.ToLower(activeClient) {
				conn = c
				found = true
				matchedKey = name
				break
			}
		}
	}

	if !found {
		var avail []string
		for name := range cfg.Connections {
			avail = append(avail, fmt.Sprintf("'%s'", name))
		}
		state.ConfigFileError = fmt.Errorf("cliente ativo '%s' não encontrado. Suas conexões configuradas no gokit.json são: %s", activeClient, strings.Join(avail, ", "))
		return state
	}

	// Atualiza com os valores reais encontrados
	state.ActiveClient = matchedKey
	state.ActiveDialect = conn.Dialect
	
	activeURL := conn.BuildURL()
	state.ActiveURL = activeURL

	// Executa teste físico
	err = TestDatabaseConnection(conn.Dialect, activeURL)
	if err != nil {
		state.ConnError = err
		state.ConnSuccess = false
	} else {
		state.ConnSuccess = true
	}

	return state
}
