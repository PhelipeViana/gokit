package i18n

// Textos da TUI que ainda estavam fixos em português: títulos de tela, mensagens
// de resultado e as linhas de contexto (origem/destino da operação).
// Os marcadores de formato (%s, %d) atravessam a tradução — quem chama continua
// usando Sprintf/Printf normalmente.
func init() {
	Register(Entradas{
		// ── Títulos de tela ──
		"tui_mig_options": {
			PT: "Opções de Migração:",
			ES: "Opciones de Migración:",
			EN: "Migration options:",
		},
		"tui_seed_options": {
			PT: "Opções de Seed:",
			ES: "Opciones de Seed:",
			EN: "Seed options:",
		},
		"tui_fact_options": {
			PT: "Opções de Factory:",
			ES: "Opciones de Factory:",
			EN: "Factory options:",
		},
		"tui_pick_operation": {
			PT: "Selecione o Tipo de Operação:",
			ES: "Seleccione el Tipo de Operación:",
			EN: "Select the operation type:",
		},
		"tui_pick_table": {
			PT: "Selecione a Tabela no Catálogo:",
			ES: "Seleccione la Tabla en el Catálogo:",
			EN: "Select the table from the catalog:",
		},
		"tui_pick_view": {
			PT: "Selecione a View no Catálogo:",
			ES: "Seleccione la View en el Catálogo:",
			EN: "Select the view from the catalog:",
		},
		"tui_scaffold_migration": {
			PT: "Scaffold Migration (%s)",
			ES: "Scaffold Migration (%s)",
			EN: "Migration scaffold (%s)",
		},
		"tui_populate_one": {
			PT: "Popular uma tabela",
			ES: "Poblar una tabla",
			EN: "Populate one table",
		},
		"tui_seed_from_db": {
			PT: "Gerar Seed a partir do banco",
			ES: "Generar Seed a partir de la base",
			EN: "Generate Seed from the database",
		},
		"tui_back_hint": {
			PT: "Pressione %s para voltar.",
			ES: "Pulse %s para volver.",
			EN: "Press %s to go back.",
		},

		// ── Resultados ──
		"tui_corpus_valid": {
			PT: "Corpus válido.",
			ES: "Corpus válido.",
			EN: "Corpus is valid.",
		},
		"tui_migrations_applied": {
			PT: "Migrations aplicadas.",
			ES: "Migraciones aplicadas.",
			EN: "Migrations applied.",
		},
		"tui_reload_done": {
			PT: "Reload concluído com sucesso.",
			ES: "Reload concluido con éxito.",
			EN: "Reload completed successfully.",
		},
		"tui_rollback_done": {
			PT: "Arquivos removidos e banco reconstruído.",
			ES: "Archivos eliminados y base reconstruida.",
			EN: "Files removed and database rebuilt.",
		},
		"tui_no_tables": {
			PT: "Nenhuma tabela encontrada no catálogo.",
			ES: "Ninguna tabla encontrada en el catálogo.",
			EN: "No table found in the catalog.",
		},
		"tui_no_views": {
			PT: "Nenhuma view encontrada no catálogo.",
			ES: "Ninguna view encontrada en el catálogo.",
			EN: "No view found in the catalog.",
		},

		// ── Falhas ──
		"tui_prevalidation_failed": {
			PT: "A pré-validação encontrou problemas:",
			ES: "La prevalidación encontró problemas:",
			EN: "Pre-validation found problems:",
		},
		"tui_migrate_failed": {
			PT: "Erro ao executar migrações:",
			ES: "Error al ejecutar migraciones:",
			EN: "Failed to run migrations:",
		},
		"tui_seed_failed": {
			PT: "A operação de seed falhou:",
			ES: "La operación de seed falló:",
			EN: "The seed operation failed:",
		},
		"tui_factory_failed": {
			PT: "A operação de factory falhou:",
			ES: "La operación de factory falló:",
			EN: "The factory operation failed:",
		},
		"tui_reload_failed": {
			PT: "A operação de reload falhou:",
			ES: "La operación de reload falló:",
			EN: "The reload operation failed:",
		},
		"tui_rollback_failed": {
			PT: "Rollback de desenvolvimento falhou:",
			ES: "El rollback de desarrollo falló:",
			EN: "Development rollback failed:",
		},
		"tui_rollback_prepare_failed": {
			PT: "Não foi possível preparar o rollback:\n\n",
			ES: "No se pudo preparar el rollback:\n\n",
			EN: "Could not prepare the rollback:\n\n",
		},

		// ── Atualização do binário ──
		"tui_update_available": {
			PT: "Atualização disponível\n\nVersão atual: %s\nNova versão:  %s\n\nO executável será substituído e reiniciado.\n\n[Enter/Y] Atualizar  ·  [Esc/N] Agora não",
			ES: "Actualización disponible\n\nVersión actual: %s\nNueva versión:  %s\n\nEl ejecutable será reemplazado y reiniciado.\n\n[Enter/Y] Actualizar  ·  [Esc/N] Ahora no",
			EN: "Update available\n\nCurrent version: %s\nNew version:     %s\n\nThe executable will be replaced and restarted.\n\n[Enter/Y] Update  ·  [Esc/N] Not now",
		},
		"tui_downloading": {
			PT: "Baixando GoKit %s...\n",
			ES: "Descargando GoKit %s...\n",
			EN: "Downloading GoKit %s...\n",
		},
		"tui_update_installed": {
			PT: "Atualização instalada. Reiniciando o GoKit...",
			ES: "Actualización instalada. Reiniciando GoKit...",
			EN: "Update installed. Restarting GoKit...",
		},
		"tui_update_done": {
			PT: "GoKit atualizado.",
			ES: "GoKit actualizado.",
			EN: "GoKit updated.",
		},
		"tui_update_failed": {
			PT: "Não foi possível atualizar o GoKit:",
			ES: "No se pudo actualizar GoKit:",
			EN: "Could not update GoKit:",
		},

		// ── Confirmações destrutivas ──
		"tui_confirm_delete_files": {
			PT: "Confirmação 1/2 · Excluir arquivos não comitados\n\n",
			ES: "Confirmación 1/2 · Eliminar archivos no confirmados\n\n",
			EN: "Confirmation 1/2 · Delete uncommitted files\n\n",
		},
		"tui_confirm_rebuild_db": {
			PT: "Confirmação 2/2 · Reconstruir banco development\n\nAs tabelas administradas pelo GoKit serão removidas e recriadas.\n\n[Enter/Y] Confirmar  ·  [Esc/N] Cancelar",
			ES: "Confirmación 2/2 · Reconstruir base development\n\nLas tablas administradas por GoKit serán eliminadas y recreadas.\n\n[Enter/Y] Confirmar  ·  [Esc/N] Cancelar",
			EN: "Confirmation 2/2 · Rebuild development database\n\nThe tables managed by GoKit will be dropped and recreated.\n\n[Enter/Y] Confirm  ·  [Esc/N] Cancel",
		},

		// ── Contexto da operação (origem/destino) ──
		"tui_reading_from": {
			PT: "Lendo de: %s (%s)",
			ES: "Leyendo de: %s (%s)",
			EN: "Reading from: %s (%s)",
		},
		"tui_source": {
			PT: "Origem:  %s (%s)\n",
			ES: "Origen:  %s (%s)\n",
			EN: "Source:  %s (%s)\n",
		},
		"tui_table": {
			PT: "Tabela:  %s\n",
			ES: "Tabla:   %s\n",
			EN: "Table:   %s\n",
		},
		"tui_rows": {
			PT: "Linhas:  %d (retrato do banco)\n",
			ES: "Filas:   %d (retrato de la base)\n",
			EN: "Rows:    %d (database snapshot)\n",
		},
		"tui_file": {
			PT: "Arquivo: %s\n",
			ES: "Archivo: %s\n",
			EN: "File:    %s\n",
		},
		"tui_factory_truncates": {
			PT: "Popular limpa a tabela antes de inserir. Destino: %s (%s).",
			ES: "Poblar limpia la tabla antes de insertar. Destino: %s (%s).",
			EN: "Populating clears the table before inserting. Target: %s (%s).",
		},
		"tui_factory_deps": {
			PT: "As tabelas de que ela depende entram junto, na ordem certa.",
			ES: "Las tablas de las que depende entran junto, en el orden correcto.",
			EN: "The tables it depends on come along, in the right order.",
		},
		"tui_seed_from_db_hint": {
			PT: "O seed fixo é lido de %s (%s) e gravado na migration que cria a tabela.",
			ES: "El seed fijo se lee de %s (%s) y se graba en la migración que crea la tabla.",
			EN: "The fixed seed is read from %s (%s) and written into the migration that creates the table.",
		},
		"tui_seed_next_steps": {
			PT: "Depois: gokit seed validate  e  gokit seed run",
			ES: "Después: gokit seed validate  y  gokit seed run",
			EN: "Next: gokit seed validate  and  gokit seed run",
		},

		// ── Status (doctor / ambiente) ──
		"tui_skeleton_created": {
			PT: "Esqueleto criado — preencha os valores.",
			ES: "Esqueleto creado — complete los valores.",
			EN: "Skeleton created — fill in the values.",
		},
		"tui_env_loaded": {
			PT: "ambiente carregado: ",
			ES: "ambiente cargado: ",
			EN: "environment loaded: ",
		},
		"tui_env_invalid": {
			PT: "arquivo de ambiente ausente ou inválido: ",
			ES: "archivo de ambiente ausente o inválido: ",
			EN: "environment file missing or invalid: ",
		},
		"tui_db_connected": {
			PT: "conexão com o banco estabelecida",
			ES: "conexión con la base establecida",
			EN: "database connection established",
		},
	})
}

// Telas da leitura do banco na TUI (scan e import).
func init() {
	Register(Entradas{
		"tui_scan_done": {
			PT: "Banco lido",
			ES: "Base leída",
			EN: "Database read",
		},
		"tui_scan_failed": {
			PT: "Falha ao ler o banco",
			ES: "Fallo al leer la base",
			EN: "Failed to read the database",
		},
		"tui_import_preview": {
			PT: "Prévia do import — nada foi escrito ainda",
			ES: "Vista previa del import — nada fue escrito aún",
			EN: "Import preview — nothing written yet",
		},
		"tui_import_done": {
			PT: "Migrations escritas",
			ES: "Migrations escritas",
			EN: "Migrations written",
		},
		"tui_import_failed": {
			PT: "Falha no import",
			ES: "Fallo en el import",
			EN: "Import failed",
		},
		"tui_confirm_import": {
			PT: "Escrever esses arquivos de migration?\n\n[Enter/Y] Escrever  ·  [Esc/N] Cancelar",
			ES: "¿Escribir esos archivos de migration?\n\n[Enter/Y] Escribir  ·  [Esc/N] Cancelar",
			EN: "Write these migration files?\n\n[Enter/Y] Write  ·  [Esc/N] Cancel",
		},
	})
}
