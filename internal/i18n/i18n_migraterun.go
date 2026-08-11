package i18n

// Mensagens dos utilitários do migraterun: fresh reload (a operação destrutiva
// de desenvolvimento), doctor (versão mínima por banco), diário de geração e a
// documentação automática gerada em Markdown.
func init() {
	Register(Entradas{
		// ── Fresh reload ──
		"frs_blocked_env": {
			PT: "fresh reload bloqueado: APP_ENV precisa ser development",
			ES: "fresh reload bloqueado: APP_ENV debe ser development",
			EN: "fresh reload blocked: APP_ENV must be development",
		},
		"frs_config_missing": {
			PT: "configuração ausente",
			ES: "configuración ausente",
			EN: "configuration missing",
		},
		"frs_conn_not_found": {
			PT: "conexão ativa %q não encontrada",
			ES: "conexión activa %q no encontrada",
			EN: "active connection %q not found",
		},
		"frs_blocked_remote": {
			PT: "fresh reload bloqueado para host não local: %s",
			ES: "fresh reload bloqueado para host no local: %s",
			EN: "fresh reload blocked for a non-local host: %s",
		},
		"frs_seed_history_failed": {
			PT: "remover histórico de seeders %s: %w",
			ES: "eliminar el historial de seeders %s: %w",
			EN: "removing seeder history %s: %w",
		},
		"frs_latest_generation": {
			PT: "Geração mais recente: %s\n",
			ES: "Generación más reciente: %s\n",
			EN: "Latest generation: %s\n",
		},
		"frs_nothing_removable": {
			PT: "nenhum arquivo removível encontrado",
			ES: "ningún archivo removible encontrado",
			EN: "no removable file found",
		},
		"frs_preview_done": {
			PT: "\nPreview concluído. Para executar use --confirm-delete --confirm-fresh.",
			ES: "\nPreview concluido. Para ejecutar use --confirm-delete --confirm-fresh.",
			EN: "\nPreview finished. To execute, use --confirm-delete --confirm-fresh.",
		},

		// ── Doctor: versão mínima por banco ──
		"doc_min_postgres": {
			PT: "O PostgreSQL deve ser versão 12 ou superior.",
			ES: "PostgreSQL debe ser versión 12 o superior.",
			EN: "PostgreSQL must be version 12 or newer.",
		},
		"doc_min_mysql": {
			PT: "O MySQL deve ser versão 8.0 ou superior.",
			ES: "MySQL debe ser versión 8.0 o superior.",
			EN: "MySQL must be version 8.0 or newer.",
		},
		"doc_min_oracle": {
			PT: "O Oracle Database deve ser versão 19c ou superior.",
			ES: "Oracle Database debe ser versión 19c o superior.",
			EN: "Oracle Database must be version 19c or newer.",
		},
		"doc_min_sqlserver": {
			PT: "O SQL Server deve ser versão 2017 (14.x) ou superior.",
			ES: "SQL Server debe ser versión 2017 (14.x) o superior.",
			EN: "SQL Server must be version 2017 (14.x) or newer.",
		},
		"doc_version_unknown": {
			PT: "Desconhecida (Falha ao consultar versão: ",
			ES: "Desconocida (Fallo al consultar la versión: ",
			EN: "Unknown (failed to query the version: ",
		},

		// ── Diário de geração ──
		"gen_outside_project": {
			PT: "arquivo gerado fora do projeto: %s",
			ES: "archivo generado fuera del proyecto: %s",
			EN: "generated file outside the project: %s",
		},
		"gen_nothing_registered": {
			PT: "nenhum arquivo gerado foi registrado",
			ES: "ningún archivo generado fue registrado",
			EN: "no generated file was registered",
		},
		"gen_already_tracked": {
			PT: " (já rastreado pelo Git)",
			ES: " (ya rastreado por Git)",
			EN: " (already tracked by Git)",
		},
		"gen_unsafe_path": {
			PT: "caminho inseguro no diário: %s",
			ES: "ruta insegura en el diario: %s",
			EN: "unsafe path in the journal: %s",
		},

		// ── Documentação automática (Markdown gerado) ──
		"dcs_config_missing": {
			PT: "gokit.json não carregado",
			ES: "gokit.json no cargado",
			EN: "gokit.json not loaded",
		},
		"dcs_mkdir_failed": {
			PT: "criar diretório de documentação: %w",
			ES: "crear el directorio de documentación: %w",
			EN: "creating the documentation directory: %w",
		},
		"dcs_generate_failed": {
			PT: "gerar %s: %w",
			ES: "generar %s: %w",
			EN: "generating %s: %w",
		},
		"dcs_publish_failed": {
			PT: "publicar %s: %w",
			ES: "publicar %s: %w",
			EN: "publishing %s: %w",
		},
		"dcs_schema_title": {
			PT: "# 🗄️ Documentação do Schema de Banco de Dados\n\n",
			ES: "# 🗄️ Documentación del Schema de Base de Datos\n\n",
			EN: "# 🗄️ Database Schema Documentation\n\n",
		},
		"dcs_schema_note": {
			PT: "> [!NOTE]\n> Documento gerado automaticamente a partir das migrations Go. Não edite manualmente.\n>\n",
			ES: "> [!NOTE]\n> Documento generado automáticamente a partir de las migraciones Go. No lo edite manualmente.\n>\n",
			EN: "> [!NOTE]\n> Document generated automatically from the Go migrations. Do not edit it by hand.\n>\n",
		},
		"dcs_updated_at": {
			PT: "> 🕒 **Última atualização:** `",
			ES: "> 🕒 **Última actualización:** `",
			EN: "> 🕒 **Last updated:** `",
		},
		"dcs_table_heading": {
			PT: "\n## 📋 Tabela: `%s`\n\n",
			ES: "\n## 📋 Tabla: `%s`\n\n",
			EN: "\n## 📋 Table: `%s`\n\n",
		},
		"dcs_table_summary": {
			PT: "> 📊 **Resumo:** %d colunas | %d restrições\n\n",
			ES: "> 📊 **Resumen:** %d columnas | %d restricciones\n\n",
			EN: "> 📊 **Summary:** %d columns | %d constraints\n\n",
		},
		"dcs_columns_header": {
			PT: "### 📌 Colunas\n\n| Nº | Campo | Descrição | Obrigatório | Tipo de dado | Chave |\n|---:|:---|:---|:---:|:---|:---:|\n",
			ES: "### 📌 Columnas\n\n| Nº | Campo | Descripción | Obligatorio | Tipo de dato | Clave |\n|---:|:---|:---|:---:|:---|:---:|\n",
			EN: "### 📌 Columns\n\n| # | Field | Description | Required | Data type | Key |\n|---:|:---|:---|:---:|:---|:---:|\n",
		},
		"dcs_constraints_header": {
			PT: "\n### 🔒 Restrições\n\n",
			ES: "\n### 🔒 Restricciones\n\n",
			EN: "\n### 🔒 Constraints\n\n",
		},
		"dcs_unidentified": {
			PT: "Não identificado",
			ES: "No identificado",
			EN: "Not identified",
		},
		"dcs_history_title": {
			PT: "# ⚙️ Histórico de Migrations\n\n",
			ES: "# ⚙️ Historial de Migraciones\n\n",
			EN: "# ⚙️ Migration History\n\n",
		},
		"dcs_history_note": {
			PT: "> [!NOTE]\n> Documento gerado automaticamente. As migrations estão em ordem decrescente de criação.\n>\n",
			ES: "> [!NOTE]\n> Documento generado automáticamente. Las migraciones están en orden descendente de creación.\n>\n",
			EN: "> [!NOTE]\n> Document generated automatically. The migrations are in descending order of creation.\n>\n",
		},
		"dcs_not_committed": {
			PT: " · não commitada",
			ES: " · no confirmada",
			EN: " · not committed",
		},
		"dcs_migration_meta": {
			PT: "  - **Criada por:** %s\n  - **Criada em:** `%s`\n  - **ID:** `%s`\n  - **Checksum:** `%s`\n",
			ES: "  - **Creada por:** %s\n  - **Creada en:** `%s`\n  - **ID:** `%s`\n  - **Checksum:** `%s`\n",
			EN: "  - **Created by:** %s\n  - **Created at:** `%s`\n  - **ID:** `%s`\n  - **Checksum:** `%s`\n",
		},
		"dcs_operations_label": {
			PT: "  - **Operações:**\n",
			ES: "  - **Operaciones:**\n",
			EN: "  - **Operations:**\n",
		},
		"dcs_op_column": {
			PT: " — coluna `%s`",
			ES: " — columna `%s`",
			EN: " — column `%s`",
		},
		"dcs_op_columns": {
			PT: " — %d coluna(s)",
			ES: " — %d columna(s)",
			EN: " — %d column(s)",
		},
	})
}
