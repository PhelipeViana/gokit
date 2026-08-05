package config

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"

	// Drivers de banco de dados
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"
)

// Scaffold padrão de gokit.yaml
const DefaultScaffoldYAML = `# Ambiente e variável
environment:
  mapper_env: ./.env #endereco do .env que sera mapeado aqui
  ambient: APP_ENV #variavel do .env que define o ambiente
  client: DB_CLIENT #variavel do .env que define qual client é o atual

# DB_CLIENT escolhe o cliente; dialect escolhe driver e linguagem SQL.
# É possível cadastrar vários clientes usando o mesmo dialect.
connections:
  client_postgres:
    dialect: postgres
    url: postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=${POSTGRES_SSLMODE:-disable}
    schema: ${POSTGRES_SCHEMA:-public}

  client_oracle:
    dialect: oracle
    url: oracle://${ORACLE_USER}:${ORACLE_PASSWORD}@${ORACLE_HOST}:${ORACLE_PORT}/${ORACLE_SERVICE}
    schema: ${ORACLE_SCHEMA:-AGENDAGOKIT}

  client_mysql:
    dialect: mysql
    url: ${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci

  client_sqlserver:
    dialect: sqlserver
    url: sqlserver://${MSSQL_USER}:${MSSQL_SA_PASSWORD}@${MSSQL_HOST}:${MSSQL_PORT}?database=${MSSQL_DATABASE}&encrypt=disable
    schema: ${MSSQL_SCHEMA:-dbo}

# Destinos gerados e/ou mapeados sempre relativos à raiz do projeto.
output:
  settings: internal/
  orm: gokit/orm
  migrate: gokit/migrate
  factory: gokit/factory
  seed: gokit/seed
  docs: gokit/docs

#preferencias internas do metodo
migrate:
  table: migrations_gokit

#preferencias internas do metodo
seed:
  table: seeders_gokit

factory:
  expressions:
    mappers:
      tabela.campo: gokit.FakeMetodo(index, 0, 1)
      
# Credenciais permanecem vazias no scaffold e são lidas do .env local.
notifications:
  slack:
    enabled: false
    webhook_url: ${SLACK_WEBHOOK_URL}
`

// Definições de estruturas para carregar o gokit.yaml
type Config struct {
	Environment   EnvConfig           `yaml:"environment"`
	Connections   map[string]ConnConfig `yaml:"connections"`
	Output        OutputConfig        `yaml:"output"`
	Migrate       MigrateConfig       `yaml:"migrate"`
	Seed          SeedConfig          `yaml:"seed"`
	Factory       FactoryConfig       `yaml:"factory"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

type EnvConfig struct {
	MapperEnv string `yaml:"mapper_env"`
	Ambient   string `yaml:"ambient"`
	Client    string `yaml:"client"`
}

type ConnConfig struct {
	Dialect string `yaml:"dialect"`
	URL     string `yaml:"url"`
	Schema  string `yaml:"schema,omitempty"`
}

type OutputConfig struct {
	Settings string `yaml:"settings"`
	ORM      string `yaml:"orm"`
	Migrate  string `yaml:"migrate"`
	Factory  string `yaml:"factory"`
	Seed     string `yaml:"seed"`
	Docs     string `yaml:"docs"`
}

type MigrateConfig struct {
	Table string `yaml:"table"`
}

type SeedConfig struct {
	Table string `yaml:"table"`
}

type FactoryConfig struct {
	Expressions FactoryExpressions `yaml:"expressions"`
}

type FactoryExpressions struct {
	Mappers map[string]string `yaml:"mappers"`
}

type NotificationsConfig struct {
	Slack SlackConfig `yaml:"slack"`
}

type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

type ConfigState struct {
	ScaffoldCreated bool
	ConfigPath      string
	ConfigFileError error
	Config          *Config
	ActiveClient    string
	ActiveDialect   string
	ActiveURL       string
	ConnError       error
	ConnSuccess     bool
}

// LoadEnvFile lê o arquivo .env e joga os valores no ambiente do sistema
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

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

		// Remove aspas
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		_ = os.Setenv(key, val)
	}
	return scanner.Err()
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

// EnsureConfigExistsAndLoad lê o gokit.yaml, criando a pasta/arquivo se não existirem
func EnsureConfigExistsAndLoad() (*Config, string, error) {
	configPath := filepath.Join("internal", "gokit.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll("internal", 0o755)
		if err != nil {
			return nil, configPath, fmt.Errorf("falha ao criar pasta internal: %v", err)
		}
		err = os.WriteFile(configPath, []byte(DefaultScaffoldYAML), 0o644)
		if err != nil {
			return nil, configPath, fmt.Errorf("falha ao gerar gokit.yaml: %v", err)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, fmt.Errorf("não foi possível ler o arquivo: %v", err)
	}

	var rawConfig Config
	err = yaml.Unmarshal(data, &rawConfig)
	if err != nil {
		return nil, configPath, fmt.Errorf("sintaxe YAML inválida: %v", err)
	}

	// Carrega arquivo .env se mapeado
	envPath := rawConfig.Environment.MapperEnv
	if envPath != "" {
		_ = LoadEnvFile(envPath)
	}

	// Substitui variáveis de ambiente nas conexões
	interpolatedConfig := rawConfig
	interpolatedConfig.Connections = make(map[string]ConnConfig)
	for name, conn := range rawConfig.Connections {
		interpolatedConfig.Connections[name] = ConnConfig{
			Dialect: conn.Dialect,
			URL:     ExpandEnvWithDefaults(conn.URL),
			Schema:  ExpandEnvWithDefaults(conn.Schema),
		}
	}
	interpolatedConfig.Notifications.Slack.WebhookURL = ExpandEnvWithDefaults(rawConfig.Notifications.Slack.WebhookURL)

	return &interpolatedConfig, configPath, nil
}

// TestDatabaseConnection abre conexão e faz ping
func TestDatabaseConnection(dialect, connURL string) error {
	driverName := dialect
	if dialect == "postgres" {
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

	configPath := filepath.Join("internal", "gokit.yaml")
	_, statErr := os.Stat(configPath)
	state.ScaffoldCreated = os.IsNotExist(statErr)

	cfg, path, err := EnsureConfigExistsAndLoad()
	state.ConfigPath = path
	if err != nil {
		state.ConfigFileError = err
		return state
	}
	state.Config = cfg

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
		state.ConfigFileError = fmt.Errorf("cliente ativo '%s' não encontrado. Suas conexões configuradas no gokit.yaml são: %s", activeClient, strings.Join(avail, ", "))
		return state
	}

	// Atualiza com os valores reais encontrados
	state.ActiveClient = matchedKey
	state.ActiveDialect = conn.Dialect
	state.ActiveURL = conn.URL

	// Executa teste físico
	err = TestDatabaseConnection(conn.Dialect, conn.URL)
	if err != nil {
		state.ConnError = err
		state.ConnSuccess = false
	} else {
		state.ConnSuccess = true
	}

	return state
}
