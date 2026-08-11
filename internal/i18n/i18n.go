package i18n

import (
	"fmt"
	"sort"
	"strings"
)

// Language representa o idioma ativo no GoKit
type Language string

const (
	PT Language = "pt"
	ES Language = "es"
	EN Language = "en"
)

var activeLanguage = PT

// SetLanguage define o idioma ativo
func SetLanguage(lang string) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "es":
		activeLanguage = ES
	case "en":
		activeLanguage = EN
	default:
		activeLanguage = PT
	}
}

// GetLanguage retorna o idioma ativo
func GetLanguage() Language {
	return activeLanguage
}

// UIString mapeia chaves de tradução para textos da interface TUI/CLI
var translations = map[string]map[Language]string{
	// Menu Principal
	"menu_config": {
		PT: "🔍 Doctor (Diagnóstico)",
		ES: "🔍 Doctor (Diagnóstico)",
		EN: "🔍 Doctor (Diagnostics)",
	},
	"menu_migrations": {
		PT: "⚙️  Opções de Migração (Migrations)",
		ES: "⚙️  Opciones de Migración (Migrations)",
		EN: "⚙️  Migration Options",
	},
	"menu_seeds": {
		PT: "🌱 Opções de Seeders (Seeds)",
		ES: "🌱 Opciones de Seeders (Seeds)",
		EN: "🌱 Seed Options",
	},
	"menu_factories": {
		PT: "🏭 Opções de Factories",
		ES: "🏭 Opciones de Factories",
		EN: "🏭 Factory Options",
	},
	"menu_reload": {
		PT: "🔄 Reload (Atualizar Projeto)",
		ES: "🔄 Reload (Actualizar Proyecto)",
		EN: "🔄 Reload (Update Project)",
	},
	"menu_exit": {
		PT: "Sair (Exit)",
		ES: "Salir (Exit)",
		EN: "Exit",
	},

	// Menu Principal em Produção
	"menu_prod_update": {
		PT: "Atualizar Banco de Dados (Apply Migrations)",
		ES: "Actualizar Base de Datos (Apply Migrations)",
		EN: "Update Database (Apply Migrations)",
	},

	// Migrations Menu
	"mig_create": {
		PT: "Criar nova Migration",
		ES: "Crear nueva Migración",
		EN: "Create new Migration",
	},
	"mig_validate": {
		PT: "Validar Migrations (não toca no banco)",
		ES: "Validar Migraciones (no toca la base)",
		EN: "Validate Migrations (read-only)",
	},
	"mig_run": {
		PT: "Executar Migrations pendentes",
		ES: "Ejecutar Migraciones pendientes",
		EN: "Run pending Migrations",
	},
	"mig_rollback": {
		PT: "Desfazer arquivos gerados e reconstruir banco (Development)",
		ES: "Deshacer archivos generados y reconstruir base (Development)",
		EN: "Remove generated files and rebuild database (Development)",
	},
	"mig_back": {
		PT: "Voltar ao menu principal",
		ES: "Volver al menú principal",
		EN: "Back to main menu",
	},

	// Seeds Menu
	"seed_create": {
		PT: "Criar Seeder de uma tabela",
		ES: "Crear Seeder de una tabla",
		EN: "Create Table Seeder",
	},
	"seed_validate": {
		PT: "Validar Seeders (não toca no banco)",
		ES: "Validar Seeders (no toca la base)",
		EN: "Validate Seeders (read-only)",
	},
	"seed_run": {
		PT: "Executar Seeders pendentes",
		ES: "Ejecutar Seeders pendientes",
		EN: "Run pending Seeders",
	},

	// Factories Menu
	"fact_create": {
		PT: "Gerar Factories a partir das Migrations",
		ES: "Generar Factories a partir de las Migraciones",
		EN: "Generate Factories from Migrations",
	},
	"fact_validate": {
		PT: "Validar Factories (não toca no banco)",
		ES: "Validar Factories (no toca la base)",
		EN: "Validate Factories (read-only)",
	},
	"fact_run_all": {
		PT: "Popular todas as tabelas ativas",
		ES: "Poblar todas las tablas activas",
		EN: "Populate all active tables",
	},
	"fact_run_one": {
		PT: "Popular uma tabela (traz as dependências)",
		ES: "Poblar una tabla (trae dependencias)",
		EN: "Populate single table (with dependencies)",
	},

	// Doutor / Diagnóstico
	"doc_title": {
		PT: "[Doctor - Diagnóstico de Prontidão do Ambiente]",
		ES: "[Doctor - Diagnóstico de Disponibilidad de Ambiente]",
		EN: "[Doctor - Environment Readiness Diagnostics]",
	},
	"doc_connectivity": {
		PT: "Conectividade: ",
		ES: "Conectividad: ",
		EN: "Connectivity: ",
	},
	"doc_env_loaded": {
		PT: "Ambiente (.env): Carregado",
		ES: "Ambiente (.env): Cargado",
		EN: "Environment (.env): Loaded",
	},
	"doc_env_warnings": {
		PT: "Ambiente (.env): Carregado, mas contém variáveis duplicadas: ",
		ES: "Ambiente (.env): Cargado, pero contiene variables duplicadas: ",
		EN: "Environment (.env): Loaded, but contains duplicate variables: ",
	},
	"doc_env_missing": {
		PT: "Ambiente (.env): Arquivo não encontrado em ",
		ES: "Ambiente (.env): Archivo no encontrado en ",
		EN: "Environment (.env): File not found at ",
	},
	"doc_conn_success": {
		PT: "Conectado com sucesso",
		ES: "Conectado con éxito",
		EN: "Connected successfully",
	},
	"doc_conn_fail": {
		PT: "Desconectado. Erro: ",
		ES: "Desconectado. Error: ",
		EN: "Disconnected. Error: ",
	},
	"doc_db_version": {
		PT: "Versão do Banco: ",
		ES: "Versión de la Base: ",
		EN: "Database Version: ",
	},
	"doc_version_alert": {
		PT: "⚠️ ALERTA: Versão do banco legada/antiga detectada. Pode haver incompatibilidade de sintaxe DDL.",
		ES: "⚠️ ALERTA: Versión de la base heredada/antigua detectada. Puede haber incompatibilidad de sintaxis DDL.",
		EN: "⚠️ WARNING: Legacy/old database version detected. Potential DDL syntax incompatibility.",
	},
}

// Entradas é um bloco de traduções no formato chave → idioma → texto. É o
// formato único do gokit: para corrigir uma frase, procure a chave; para somar
// um idioma, acrescente a linha em cada chave.
type Entradas map[string]map[Language]string

// Register incorpora um bloco de traduções ao catálogo. Cada arquivo de domínio
// (i18n_gen_orm.go, i18n_gen_response.go, ...) chama Register no seu init(),
// então o catálogo cresce sem que este arquivo precise mudar.
func Register(entradas Entradas) {
	for chave, idiomas := range entradas {
		translations[chave] = idiomas
	}
}

// Keys devolve as chaves registradas em ordem — usado por auditoria/testes para
// achar chave sem tradução em algum idioma.
func Keys() []string {
	chaves := make([]string, 0, len(translations))
	for chave := range translations {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)
	return chaves
}

// T retorna a string traduzida correspondente ao idioma ativo.
func T(key string) string {
	if langs, ok := translations[key]; ok {
		if val, ok := langs[activeLanguage]; ok {
			return val
		}
		// Fallback para inglês
		if val, ok := langs[EN]; ok {
			return val
		}
	}
	return key
}

// Tf traduz a chave e aplica os argumentos nos marcadores da frase (%s, %d, %q).
// Use sempre que a mensagem tiver partes variáveis, em vez de concatenar: assim
// cada idioma pode colocar o valor onde a frase dele precisa.
func Tf(chave string, args ...any) string {
	return fmt.Sprintf(T(chave), args...)
}

// Errf devolve um error com a mensagem traduzida. A frase pode usar %w para
// embrulhar a causa — errors.Is/errors.As continuam funcionando através dela.
//
// Preferir este helper a fmt.Errorf(i18n.T(...)) tem um motivo prático: o vet
// reclama de formato não constante em fmt.Errorf sem argumentos, e aqui a
// chamada nunca é analisada como printf.
func Errf(chave string, args ...any) error {
	return fmt.Errorf(T(chave), args...)
}

// Mapeamento de escrita e geração de códigos/métodos de scaffold
var methodTranslations = map[string]map[Language]string{
	"Get": {
		PT: "Pegar",
		ES: "Obtener",
		EN: "Get",
	},
	"Filter": {
		PT: "Filtro",
		ES: "Filtro",
		EN: "Filter",
	},
	"Delete": {
		PT: "Deletar",
		ES: "Borrar",
		EN: "Delete",
	},
	"Create": {
		PT: "Criar",
		ES: "Crear",
		EN: "Create",
	},
}

// MethodName retorna o termo traduzido para geração de scaffolds e métodos no core.
func MethodName(key string) string {
	if langs, ok := methodTranslations[key]; ok {
		if val, ok := langs[activeLanguage]; ok {
			return val
		}
	}
	return key
}
