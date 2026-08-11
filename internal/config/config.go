package config

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/gomodule"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"

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
    "migrate": "",
    "factory": "",
    "seed": "",
    "docs": ""
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
	Language      string                `json:"language,omitempty"`
	Go            GoConfig              `json:"go,omitempty"`
	Environment   EnvConfig             `json:"environment"`
	Connections   map[string]ConnConfig `json:"connections"`
	Output        OutputConfig          `json:"output"`
	Migrate       MigrateConfig         `json:"migrate"`
	Seed          SeedConfig            `json:"seed"`
	Factory       FactoryConfig         `json:"factory"`
	Notifications NotificationsConfig   `json:"notifications"`
}

type GoConfig struct {
	Module string `json:"module,omitempty"`
	// Mode decide como o projeto consome o gokit: "dev" usa o checkout da pasta
	// irmã por go.work, "prod" fixa a versão publicada. Escolhido na criação.
	Mode            string `json:"mode,omitempty"`
	GoKitModule     string `json:"gokit_module,omitempty"`
	GoKitVersion    string `json:"gokit_version,omitempty"`
	GoKitLocal      string `json:"gokit_local,omitempty"`
	Execution       string `json:"execution,omitempty"`
	DockerService   string `json:"docker_service,omitempty"`
	DockerAutoStart *bool  `json:"docker_auto_start,omitempty"`
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

// checkFolderEmpty verifica se o projeto está vazio (sem migrations e sem outros códigos Go)
func checkFolderEmpty() bool {
	goFilesFound := false
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
			// Se for main.go na raiz do gokit, ignora
			if !strings.Contains(path, "internal/gokit") && info.Name() != "main.go" {
				goFilesFound = true
			}
		}
		return nil
	})

	if goFilesFound {
		return false
	}

	if _, err := os.Stat(filepath.Join("database", "migrations")); err == nil {
		return false
	}
	if _, err := os.Stat(filepath.Join("internal", "gokit", "gokit.json")); err == nil {
		return false
	}
	return true
}

func createOnboardingScaffold() error {
	// O modo da criação: dev quando o gokit está na pasta irmã, prod quando não
	// está. É palpite só aqui — a partir deste ponto o valor fica declarado em
	// go.mode no gokit.json, e é ele que manda.
	modo := gomodule.ModoInferido(".", gomodule.CanonicalGoKitModule)
	moduleResult, err := gomodule.Ensure(gomodule.Options{
		Root:        ".",
		GoKitModule: gomodule.CanonicalGoKitModule,
		GoKitLocal:  "auto",
		Mode:        modo,
	})
	if err != nil {
		return i18n.Errf("cfg_gomod_init_failed", err)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	mysqlPort := 20000 + r.Intn(10000)
	postgresPort := 20000 + r.Intn(10000)
	oraclePort := 20000 + r.Intn(10000)
	mssqlPort := 20000 + r.Intn(10000)

	envContent := fmt.Sprintf(`# Dialeto ativo do gokit (mysql | postgres | oracle | sqlserver)
DB_DIALECT=mysql
APP_ENV=development

# === MySQL (Ativo por padrão) ===
MYSQL_HOST=127.0.0.1
MYSQL_PORT=%d
MYSQL_DATABASE=gokit_onboarding
MYSQL_USER=gokit_user
MYSQL_PASSWORD=gokit_password
MYSQL_ROOT_PASSWORD=gokit_root_password
MYSQL_SCHEMA=gokit_onboarding

# === Outros dialetos (comentados para fins didáticos) ===
# POSTGRES_HOST=127.0.0.1
# POSTGRES_PORT=%d
# POSTGRES_DB=gokit_onboarding
# POSTGRES_USER=gokit_user
# POSTGRES_PASSWORD=gokit_password
# POSTGRES_SCHEMA=public
# POSTGRES_SSLMODE=disable

# ORACLE_HOST=127.0.0.1
# ORACLE_PORT=%d
# ORACLE_USER=gokit_user
# ORACLE_PASSWORD=gokit_password
# ORACLE_SERVICE=FREEPDB1
# ORACLE_SCHEMA=GOKIT_ONBOARDING

# MSSQL_HOST=127.0.0.1
# MSSQL_PORT=%d
# MSSQL_USER=sa
# MSSQL_DATABASE=gokit_onboarding
# MSSQL_SA_PASSWORD=Gokit_password123!
# MSSQL_SCHEMA=dbo
`, mysqlPort, postgresPort, oraclePort, mssqlPort)

	if err := os.WriteFile(".env", []byte(envContent), 0o644); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join("internal", "gokit"), 0o755); err != nil {
		return err
	}

	gokitJSON := fmt.Sprintf(`{
  "language": "pt",
  "go": {
    "module": %q,
    "mode": %q,
    "gokit_module": %q,
    "gokit_version": %q,
    "gokit_local": %q,
    "execution": "docker",
    "docker_service": "toolchain",
    "docker_auto_start": true
  },
  "environment": {
    "mapper_env": ".env",
    "ambient": "APP_ENV",
    "client": "DB_DIALECT"
  },
  "connections": {
    "mysql": {
      "dialect": "mysql",
      "host": "${MYSQL_HOST:-127.0.0.1}",
      "port": "${MYSQL_PORT:-3306}",
      "user": "${MYSQL_USER:-root}",
      "password": "${MYSQL_PASSWORD}",
      "database": "${MYSQL_DATABASE:-gokit_onboarding}"
    },
    "postgres": {
      "dialect": "postgres",
      "host": "${POSTGRES_HOST:-127.0.0.1}",
      "port": "${POSTGRES_PORT:-5432}",
      "user": "${POSTGRES_USER:-postgres}",
      "password": "${POSTGRES_PASSWORD}",
      "database": "${POSTGRES_DB:-gokit_onboarding}",
      "schema": "${POSTGRES_SCHEMA:-public}"
    },
    "oracle": {
      "dialect": "oracle",
      "host": "${ORACLE_HOST:-127.0.0.1}",
      "port": "${ORACLE_PORT:-1521}",
      "user": "${ORACLE_USER:-system}",
      "password": "${ORACLE_PASSWORD}",
      "service": "${ORACLE_SERVICE:-FREEPDB1}",
      "schema": "${ORACLE_SCHEMA:-GOKIT_ONBOARDING}"
    },
    "sqlserver": {
      "dialect": "sqlserver",
      "host": "${MSSQL_HOST:-127.0.0.1}",
      "port": "${MSSQL_PORT:-1433}",
      "user": "${MSSQL_USER:-sa}",
      "password": "${MSSQL_SA_PASSWORD}",
      "database": "${MSSQL_DATABASE:-gokit_onboarding}",
      "schema": "${MSSQL_SCHEMA:-dbo}"
    }
  },
  "output": {
    "migrate": "",
    "factory": "",
    "seed": "",
    "docs": ""
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
  }
}`, moduleResult.Module, moduleResult.Mode, moduleResult.GoKitModule, moduleResult.GoKitVersion, moduleResult.GoKitLocal)
	if err := os.WriteFile(filepath.Join("internal", "gokit", "gokit.json"), []byte(gokitJSON), 0o644); err != nil {
		return err
	}

	// Gera as configurações do Docker em sincronia com as portas do .env
	// O bind mount do gokit local só existe no modo dev: em prod ele seria um
	// caminho de fora do projeto num arquivo commitado, que só funciona na
	// máquina de quem gerou.
	localGoKitVolume := ""
	if moduleResult.Mode == gomodule.ModoDev && moduleResult.GoKitLocal != "" {
		localGoKitVolume = fmt.Sprintf("      - %s:/gokit\n", moduleResult.GoKitLocal)
	}
	dockerCompose := fmt.Sprintf(`services:
  toolchain:
    image: golang:1.26-alpine
    working_dir: /workspace
    volumes:
      - .:/workspace
      - go_modules:/go/pkg/mod
%s    profiles: ["tools"]

  # MySQL (Padrão Ativo)
  mysql:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: gokit_password
      MYSQL_DATABASE: gokit_onboarding
      MYSQL_USER: gokit_user
      MYSQL_PASSWORD: gokit_password
    ports:
      - "%d:3306"
    volumes:
      - mysql_data:/var/lib/mysql

  # PostgreSQL (Descomente para usar)
  # postgres:
  #   image: postgres:16-alpine
  #   environment:
  #     POSTGRES_USER: postgres
  #     POSTGRES_PASSWORD: gokit_password
  #     POSTGRES_DB: gokit_onboarding
  #   ports:
  #     - "%d:5432"
  #   volumes:
  #     - postgres_data:/var/lib/postgresql/data

  # Oracle DB (Descomente para usar)
  # oracle:
  #   image: gvenzl/oracle-free:23.4-slim
  #   environment:
  #     ORACLE_PASSWORD: gokit_password
  #   ports:
  #     - "%d:1521"
  #   volumes:
  #     - oracle_data:/opt/oracle/oradata

  # SQL Server (Descomente para usar)
  # mssql:
  #   image: mcr.microsoft.com/mssql/server:2022-latest
  #   platform: linux/amd64
  #   ulimits:
  #     memlock:
  #       soft: -1
  #       hard: -1
  #     nofile:
  #       soft: 65536
  #       hard: 65536
  #   environment:
  #     ACCEPT_EULA: "Y"
  #     MSSQL_PID: Developer
  #     MSSQL_SA_PASSWORD: "Gokit_password123!"
  #   ports:
  #     - "%d:1433"
  #   volumes:
  #     - mssql_data:/var/opt/mssql
  #   healthcheck:
  #     test: ["CMD-SHELL", "/opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P \"$${MSSQL_SA_PASSWORD}\" -C -Q \"SELECT 1\" || /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P \"$${MSSQL_SA_PASSWORD}\" -Q \"SELECT 1\""]
  #     interval: 10s
  #     timeout: 5s
  #     retries: 30
  #     start_period: 60s

volumes:
  go_modules:
  mysql_data:
  # postgres_data:
  # oracle_data:
  # mssql_data:
`, localGoKitVolume, mysqlPort, postgresPort, oraclePort, mssqlPort)
	_ = os.WriteFile("docker-compose.yml", []byte(dockerCompose), 0o644)

	dockerIgnore := `.git
.env
internal/gokit/gokit_local
internal/gokit/.state/
gokit
gokit_local
*.log
`
	_ = os.WriteFile(".dockerignore", []byte(dockerIgnore), 0o644)
	// go.work é decisão de máquina, não de projeto: cada dev aponta para o seu
	// checkout do gokit. Por isso fica fora do Git.
	gitIgnore := "internal/gokit/.state/\ngo.work\ngo.work.sum\n"
	if current, err := os.ReadFile(".gitignore"); err == nil {
		if !strings.Contains(string(current), "internal/gokit/.state/") {
			_ = os.WriteFile(".gitignore", append(current, []byte("\n"+gitIgnore)...), 0o644)
		}
	} else if os.IsNotExist(err) {
		_ = os.WriteFile(".gitignore", []byte(gitIgnore), 0o644)
	}

	// Configuração de editor no scaffold: a árvore de anotações e o executor de
	// .http já valem no primeiro dia. Falha aqui não impede o projeto de nascer.
	_, _, _ = EscreverConfigEditores(".")

	_ = os.MkdirAll(filepath.Join("internal", "gokit", "migrate", "create_table"), 0o755)
	_ = os.MkdirAll(filepath.Join("internal", "gokit", "migrate", "add_column"), 0o755)
	_ = os.MkdirAll(filepath.Join("internal", "gokit", "seed", "users"), 0o755)
	_ = os.MkdirAll(filepath.Join("internal", "gokit", "factory"), 0o755)

	cidadesMig := `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2026_08_08_000001_CreateCidadesTable() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("cidades",
			migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
			migrate.Col("nome").Varchar(255).NotNull(),
		).Alias("cidades"),
	)
}
`
	_ = os.WriteFile(filepath.Join("internal", "gokit", "migrate", "create_table", "2026_08_08_000001_create_cidades_table.go"), []byte(cidadesMig), 0o644)

	usersMig := `package migrations

import migrate "github.com/PhelipeViana/gokit/migration"

func Migration_2026_08_08_000002_CreateUsersTable() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("users",
			migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
			migrate.Col("nome").Varchar(255).NotNull(),
			migrate.Col("email").Varchar(255).NotNull().Unique(),
		).Alias("users"),
	)
}
`
	_ = os.WriteFile(filepath.Join("internal", "gokit", "migrate", "create_table", "2026_08_08_000002_create_users_table.go"), []byte(usersMig), 0o644)

	fkMig := fmt.Sprintf(`package migrations

import (
	alias %q
	migrate "github.com/PhelipeViana/gokit/migration"
)

func Migration_2026_08_08_000003_AddCidadeToUsers() migrate.Definition {
	return migrate.Define(
		migrate.AddColumn(alias.Users,
			migrate.Col("cidade_id").Integer().Nullable().References("cidades", "id").OnDeleteCascade(),
		),
	)
}
`, moduleResult.Module+"/internal/gokit/core/migration/alias")
	_ = os.WriteFile(filepath.Join("internal", "gokit", "migrate", "add_column", "2026_08_08_000003_add_cidade_to_users.go"), []byte(fkMig), 0o644)

	seederContent := `package users

import migrate "github.com/PhelipeViana/gokit/migration"

func Seeder_2026_08_08_000001_Users() migrate.Rows {
	return migrate.Rows{
		{"id": 1, "nome": "Phelipe Viana", "email": "phelipe@gokit.io"},
		{"id": 2, "nome": "Joao Silva", "email": "joao@gokit.io"},
	}
}
`
	_ = os.WriteFile(filepath.Join("internal", "gokit", "seed", "users", "2026_08_08_000001_users_seeder.go"), []byte(seederContent), 0o644)

	cidadesFact := `package factories

import migrate "github.com/PhelipeViana/gokit/migration"

func CidadesFactory() migrate.Factory {
	return migrate.Factory{
		Table: "CIDADES",
		Ruler: migrate.Ruler{Count: 10, Update: true, Active: true},
		Data: func(index int) migrate.Fields {
			return migrate.Fields{
				"id":   migrate.FakeIntIndex(index, 1, 99999999),
				"nome": migrate.FakeCityIndexLength(index, 100),
			}
		},
	}
}
`
	_ = os.WriteFile(filepath.Join("internal", "gokit", "factory", "cidades_factory.go"), []byte(cidadesFact), 0o644)

	usersFact := `package factories

import migrate "github.com/PhelipeViana/gokit/migration"

func UsersFactory() migrate.Factory {
	return migrate.Factory{
		Table: "USERS",
		Ruler: migrate.Ruler{Count: 20, Update: true, Active: true},
		Data: func(index int) migrate.Fields {
			return migrate.Fields{
				"id":        migrate.FakeIntIndex(index, 1, 99999999),
				"nome":      migrate.FakeNameIndexLength(index, 100),
				"email":     migrate.FakeEmailIndexLength(index, 100),
				"cidade_id": migrate.Vinculo("CIDADES", "ID"),
			}
		},
	}
}
`
	_ = os.WriteFile(filepath.Join("internal", "gokit", "factory", "users_factory.go"), []byte(usersFact), 0o644)

	// Atualiza o catálogo do Core para gerar o dsl.gen.go contendo os aliases Cidades e Users recém-criados
	_ = migrationgo.RefreshCatalog(".", filepath.Join("internal", "gokit", "migrate"))

	return nil
}

func readEnvAndBuildGokitJSON() string {
	envPath := ".env"
	if _, err := os.Stat("configs/api.env"); err == nil {
		envPath = "configs/api.env"
	}

	mysqlHost := "127.0.0.1"
	mysqlPort := "3306"
	mysqlUser := "root"
	mysqlDatabase := "gokit_onboarding"

	postgresHost := "127.0.0.1"
	postgresPort := "5432"
	postgresUser := "postgres"
	postgresDatabase := "postgres"

	oracleHost := "127.0.0.1"
	oraclePort := "1521"
	oracleUser := "system"
	oracleService := "FREEPDB1"

	mssqlHost := "127.0.0.1"
	mssqlPort := "1433"
	mssqlUser := "sa"
	mssqlDatabase := "master"

	if file, err := os.Open(envPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if len(line) == 0 || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
					v = v[1 : len(v)-1]
				}
				switch k {
				case "MYSQL_HOST", "DB_HOST":
					mysqlHost = v
					postgresHost = v
					oracleHost = v
					mssqlHost = v
				case "MYSQL_PORT":
					mysqlPort = v
				case "MYSQL_USER", "DB_USER":
					mysqlUser = v
					postgresUser = v
					oracleUser = v
					mssqlUser = v
				case "MYSQL_DATABASE", "DB_NAME":
					mysqlDatabase = v
					postgresDatabase = v
					mssqlDatabase = v
				case "POSTGRES_HOST":
					postgresHost = v
				case "POSTGRES_PORT":
					postgresPort = v
				case "POSTGRES_USER":
					postgresUser = v
				case "POSTGRES_DB":
					postgresDatabase = v
				case "ORACLE_HOST":
					oracleHost = v
				case "ORACLE_PORT":
					oraclePort = v
				case "ORACLE_USER":
					oracleUser = v
				case "ORACLE_SERVICE":
					oracleService = v
				case "MSSQL_HOST":
					mssqlHost = v
				case "MSSQL_PORT":
					mssqlPort = v
				case "MSSQL_USER":
					mssqlUser = v
				case "MSSQL_DATABASE":
					mssqlDatabase = v
				}
			}
		}
	}

	return fmt.Sprintf(`{
  "language": "pt",
  "environment": {
    "mapper_env": "%s",
    "ambient": "APP_ENV",
    "client": "DB_DIALECT"
  },
  "connections": {
    "mysql": {
      "dialect": "mysql",
      "host": "${MYSQL_HOST:-%s}",
      "port": "${MYSQL_PORT:-%s}",
      "user": "${MYSQL_USER:-%s}",
      "password": "${MYSQL_PASSWORD}",
      "database": "${MYSQL_DATABASE:-%s}"
    },
    "postgres": {
      "dialect": "postgres",
      "host": "${POSTGRES_HOST:-%s}",
      "port": "${POSTGRES_PORT:-%s}",
      "user": "${POSTGRES_USER:-%s}",
      "password": "${POSTGRES_PASSWORD}",
      "database": "${POSTGRES_DB:-%s}",
      "schema": "${POSTGRES_SCHEMA:-public}"
    },
    "oracle": {
      "dialect": "oracle",
      "host": "${ORACLE_HOST:-%s}",
      "port": "${ORACLE_PORT:-%s}",
      "user": "${ORACLE_USER:-%s}",
      "password": "${ORACLE_PASSWORD}",
      "service": "${ORACLE_SERVICE:-%s}",
      "schema": "${ORACLE_SCHEMA:-PREVCONTAS_TEST}"
    },
    "sqlserver": {
      "dialect": "sqlserver",
      "host": "${MSSQL_HOST:-%s}",
      "port": "${MSSQL_PORT:-%s}",
      "user": "${MSSQL_USER:-%s}",
      "password": "${MSSQL_SA_PASSWORD}",
      "database": "${MSSQL_DATABASE:-%s}",
      "schema": "${MSSQL_SCHEMA:-dbo}"
    }
  },
  "output": {
    "migrate": "",
    "factory": "",
    "seed": "",
    "docs": ""
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
  }
}`, envPath, mysqlHost, mysqlPort, mysqlUser, mysqlDatabase, postgresHost, postgresPort, postgresUser, postgresDatabase, oracleHost, oraclePort, oracleUser, oracleService, mssqlHost, mssqlPort, mssqlUser, mssqlDatabase)
}

// EnsureConfigExistsAndLoad lê o gokit.json, criando a pasta/arquivo se não existirem
func EnsureConfigExistsAndLoad() (*Config, string, []string, error) {
	configPath := filepath.Join("internal", "gokit", "gokit.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if checkFolderEmpty() {
			_ = createOnboardingScaffold()
		} else {
			_ = os.MkdirAll(filepath.Join("internal", "gokit"), 0o755)
			gokitJSON := readEnvAndBuildGokitJSON()
			_ = os.WriteFile(configPath, []byte(gokitJSON), 0o644)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, nil, i18n.Errf("cfg_read_failed", err)
	}

	var rawConfig Config
	err = json.Unmarshal(data, &rawConfig)
	if err != nil {
		return nil, configPath, nil, i18n.Errf("cfg_bad_json", err)
	}

	// Inicializa o idioma ativo baseado na configuração
	if rawConfig.Language != "" {
		i18n.SetLanguage(rawConfig.Language)
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

	// Preenche os padrões não-invasivos internos caso o usuário tenha deixado os campos vazios no gokit.json
	if interpolatedConfig.Output.Settings == "" {
		interpolatedConfig.Output.Settings = "internal/gokit/gokit.json"
	}
	if interpolatedConfig.Output.ORM == "" {
		interpolatedConfig.Output.ORM = "internal/gokit/core/orm"
	}
	if interpolatedConfig.Output.Migrate == "" {
		interpolatedConfig.Output.Migrate = "internal/gokit/migrate"
	}
	if interpolatedConfig.Output.Factory == "" {
		interpolatedConfig.Output.Factory = "internal/gokit/factory"
	}
	if interpolatedConfig.Output.Seed == "" {
		interpolatedConfig.Output.Seed = "internal/gokit/seed"
	}
	if interpolatedConfig.Output.Docs == "" {
		interpolatedConfig.Output.Docs = "internal/gokit/docs"
	}

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
	activeEnvRaw := strings.TrimSpace(os.Getenv(envKey))
	if activeEnvRaw == "" {
		for _, altKey := range []string{"APP_ENV", "ENV", "ENVIRONMENT"} {
			if val := strings.TrimSpace(os.Getenv(altKey)); val != "" {
				activeEnvRaw = val
				break
			}
		}
	}

	// Regra de segurança: se começar com 'd' ou 'D', é development.
	// Se começar com 'p' ou 'P', ou se for vazio/desconhecido, assume 'production'.
	activeEnv := "production"
	if activeEnvRaw != "" {
		firstChar := strings.ToLower(activeEnvRaw[:1])
		if firstChar == "d" {
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
		state.ConfigFileError = i18n.Errf("cfg_client_not_found", activeClient, strings.Join(avail, ", "))
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
