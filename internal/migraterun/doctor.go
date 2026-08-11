package migraterun

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
)

// DoctorReport agrupa o diagnóstico completo de uma conexão ativa
type DoctorReport struct {
	Dialect         string
	Host            string
	ConnSuccess     bool
	ConnError       error
	Version         string
	VersionOK       bool
	VersionWarning  string
	DDLSuccess      bool
	DDLError        error
}

// RunDoctor diagnostica a conexão de banco de dados ativa
func RunDoctor(state config.ConfigState) DoctorReport {
	var report DoctorReport
	report.Dialect = state.ActiveDialect
	report.Host = state.ActiveURL

	// 1. Testa conectividade
	db, err := sql.Open(getDriverName(state.ActiveDialect), state.ActiveURL)
	if err != nil {
		report.ConnError = err
		return report
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		report.ConnError = err
		return report
	}
	report.ConnSuccess = true

	// 2. Coleta e valida versão
	version, verOK, verWarning := checkDBVersion(ctx, db, state.ActiveDialect)
	report.Version = version
	report.VersionOK = verOK
	report.VersionWarning = verWarning

	// 3. Testa permissões de DDL
	ddlErr := testDDLPermissions(ctx, db, state.ActiveDialect)
	if ddlErr != nil {
		report.DDLError = ddlErr
	} else {
		report.DDLSuccess = true
	}

	return report
}

func getDriverName(dialect string) string {
	d := strings.ToLower(strings.TrimSpace(dialect))
	if d == "postgres" || d == "postgresql" {
		return "pgx"
	}
	return d
}

func checkDBVersion(ctx context.Context, db *sql.DB, dialect string) (string, bool, string) {
	d := strings.ToLower(strings.TrimSpace(dialect))
	var versionStr string
	var err error

	switch d {
	case "postgres", "postgresql":
		err = db.QueryRowContext(ctx, "SHOW server_version;").Scan(&versionStr)
		if err != nil {
			err = db.QueryRowContext(ctx, "SELECT version();").Scan(&versionStr)
		}
		if err == nil {
			major := parseMajorVersion(versionStr)
			if major >= 12 {
				return versionStr, true, ""
			}
			return versionStr, false, i18n.T("doc_min_postgres")
		}

	case "mysql":
		err = db.QueryRowContext(ctx, "SELECT version();").Scan(&versionStr)
		if err == nil {
			major := parseMajorVersion(versionStr)
			if major >= 8 {
				return versionStr, true, ""
			}
			return versionStr, false, i18n.T("doc_min_mysql")
		}

	case "oracle":
		// Query mais genérica para versão no Oracle
		err = db.QueryRowContext(ctx, "SELECT VERSION FROM PRODUCT_COMPONENT_VERSION WHERE ROWNUM = 1").Scan(&versionStr)
		if err != nil {
			err = db.QueryRowContext(ctx, "SELECT VERSION_FULL FROM V$INSTANCE").Scan(&versionStr)
		}
		if err != nil {
			err = db.QueryRowContext(ctx, "SELECT VERSION FROM V$INSTANCE").Scan(&versionStr)
		}
		if err == nil {
			major := parseMajorVersion(versionStr)
			if major >= 19 {
				return versionStr, true, ""
			}
			return versionStr, false, i18n.T("doc_min_oracle")
		}

	case "sqlserver":
		err = db.QueryRowContext(ctx, "SELECT CAST(SERVERPROPERTY('ProductVersion') AS VARCHAR)").Scan(&versionStr)
		if err == nil {
			major := parseMajorVersion(versionStr)
			if major >= 14 { // 14.x é SQL Server 2017
				return versionStr, true, ""
			}
			return versionStr, false, i18n.T("doc_min_sqlserver")
		}
	}

	if err != nil {
		return i18n.T("doc_version_unknown") + err.Error() + ")", true, ""
	}
	return "Desconhecida", true, ""
}

func parseMajorVersion(version string) int {
	re := regexp.MustCompile(`(\d+)\.`)
	matches := re.FindStringSubmatch(version)
	if len(matches) > 1 {
		val, _ := strconv.Atoi(matches[1])
		return val
	}
	// Fallback para tentar ler apenas os dígitos iniciais
	reDigits := regexp.MustCompile(`^\d+`)
	digits := reDigits.FindString(version)
	if digits != "" {
		val, _ := strconv.Atoi(digits)
		return val
	}
	return 0
}

func testDDLPermissions(ctx context.Context, db *sql.DB, dialect string) error {
	d := strings.ToLower(strings.TrimSpace(dialect))
	tableName := "gokit_doctor_ddl_test"

	// Garante que a tabela não existe
	_ = dropTableIfExists(ctx, db, d, tableName)

	// CREATE TABLE
	createSQL := fmt.Sprintf("CREATE TABLE %s (id INT)", tableName)
	if d == "oracle" {
		createSQL = fmt.Sprintf("CREATE TABLE %s (id NUMBER)", tableName)
	}
	_, err := db.ExecContext(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("falha ao criar tabela de teste (permissão de CREATE): %w", err)
	}

	// DROP TABLE
	err = dropTableIfExists(ctx, db, d, tableName)
	if err != nil {
		return fmt.Errorf("falha ao deletar tabela de teste (permissão de DROP): %w", err)
	}

	return nil
}

func dropTableIfExists(ctx context.Context, db *sql.DB, dialect, tableName string) error {
	dropSQL := fmt.Sprintf("DROP TABLE %s", tableName)
	if dialect == "oracle" {
		// Oracle não aceita IF EXISTS diretamente na DDL clássica sem bloco PL/SQL
		_, err := db.ExecContext(ctx, dropSQL)
		if err != nil && !strings.Contains(err.Error(), "ORA-00942") {
			return err
		}
		return nil
	}

	if dialect == "mysql" || dialect == "postgres" || dialect == "postgresql" {
		dropSQL = fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	}

	_, err := db.ExecContext(ctx, dropSQL)
	return err
}
