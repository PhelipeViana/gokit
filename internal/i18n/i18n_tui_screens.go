package i18n

// Segunda metade dos textos da TUI: navegação, rodapés, etiquetas curtas, telas
// de criação de migration e o checklist do doctor. Ficou em arquivo próprio para
// o i18n_tui.go não virar um bloco de 500 linhas — o catálogo é o mesmo, cada
// arquivo só acrescenta o seu pedaço no init().
func init() {
	Register(Entradas{
		// ── Navegação e rodapés ──
		"tui_counter": {
			PT: "%d de %d",
			ES: "%d de %d",
			EN: "%d of %d",
		},
		"tui_more_above": {
			PT: "  ↑ mais opções",
			ES: "  ↑ más opciones",
			EN: "  ↑ more options",
		},
		"tui_more_below": {
			PT: "  ↓ mais opções",
			ES: "  ↓ más opciones",
			EN: "  ↓ more options",
		},
		"tui_lines_omitted": {
			PT: "... (%d linha(s) acima omitidas)\n%s",
			ES: "... (%d línea(s) arriba omitidas)\n%s",
			EN: "... (%d line(s) above omitted)\n%s",
		},
		"tui_nav_confirm_generate": {
			PT: "  [Enter] Confirmar e Gerar  ·  [Esc] Voltar\n",
			ES: "  [Enter] Confirmar y Generar  ·  [Esc] Volver\n",
			EN: "  [Enter] Confirm and Generate  ·  [Esc] Back\n",
		},
		"tui_nav_next_options": {
			PT: "\n  [Enter] Avançar  ·  [Esc] Voltar para Opções\n",
			ES: "\n  [Enter] Avanzar  ·  [Esc] Volver a Opciones\n",
			EN: "\n  [Enter] Next  ·  [Esc] Back to Options\n",
		},
		"tui_nav_next_actions": {
			PT: "\n  [Enter] Avançar  ·  [Esc] Voltar para Ações\n",
			ES: "\n  [Enter] Avanzar  ·  [Esc] Volver a Acciones\n",
			EN: "\n  [Enter] Next  ·  [Esc] Back to Actions\n",
		},
		"tui_nav_confirm_actions": {
			PT: "\n  [Enter] Confirmar e Gerar  ·  [Esc] Voltar para Ações\n",
			ES: "\n  [Enter] Confirmar y Generar  ·  [Esc] Volver a Acciones\n",
			EN: "\n  [Enter] Confirm and Generate  ·  [Esc] Back to Actions\n",
		},
		"tui_nav_back_esc": {
			PT: "\n\n[Esc] Voltar",
			ES: "\n\n[Esc] Volver",
			EN: "\n\n[Esc] Back",
		},
		"tui_nav_back_main": {
			PT: "[Enter] Voltar ao menu principal",
			ES: "[Enter] Volver al menú principal",
			EN: "[Enter] Back to main menu",
		},
		"tui_confirm_cancel": {
			PT: "\n[Enter/Y] Confirmar  ·  [Esc/N] Cancelar",
			ES: "\n[Enter/Y] Confirmar  ·  [Esc/N] Cancelar",
			EN: "\n[Enter/Y] Confirm  ·  [Esc/N] Cancel",
		},
		"tui_footer_seed": {
			PT: "%d de %d  ·  [Enter] Gerar  ·  [q] Sair",
			ES: "%d de %d  ·  [Enter] Generar  ·  [q] Salir",
			EN: "%d of %d  ·  [Enter] Generate  ·  [q] Quit",
		},
		"tui_footer_factory": {
			PT: "%d de %d  ·  [Enter] Popular  ·  [q] Sair",
			ES: "%d de %d  ·  [Enter] Poblar  ·  [q] Salir",
			EN: "%d of %d  ·  [Enter] Populate  ·  [q] Quit",
		},

		// ── Rótulos curtos e etiquetas ──
		"tui_tag_error": {
			PT: "[Erro]",
			ES: "[Error]",
			EN: "[Error]",
		},
		"tui_tag_action": {
			PT: "[Ação]",
			ES: "[Acción]",
			EN: "[Action]",
		},
		"tui_done": {
			PT: "Concluído.",
			ES: "Concluido.",
			EN: "Done.",
		},
		"tui_solutions": {
			PT: "⚠️ Possíveis soluções:",
			ES: "⚠️ Posibles soluciones:",
			EN: "⚠️ Possible fixes:",
		},
		"tui_input_placeholder": {
			PT: "digite aqui...",
			ES: "escriba aquí...",
			EN: "type here...",
		},
		"tui_status_ok": {
			PT: "✅ sucesso",
			ES: "✅ éxito",
			EN: "✅ success",
		},
		"tui_status_fail": {
			PT: "❌ falha",
			ES: "❌ fallo",
			EN: "❌ failure",
		},
		"tui_env_production": {
			PT: "🚨 PRODUÇÃO (production)",
			ES: "🚨 PRODUCCIÓN (production)",
			EN: "🚨 PRODUCTION (production)",
		},
		"tui_env_local": {
			PT: "💻 LOCAL (development)",
			ES: "💻 LOCAL (development)",
			EN: "💻 LOCAL (development)",
		},
		"tui_db_unset": {
			PT: "💾 banco não configurado",
			ES: "💾 base no configurada",
			EN: "💾 database not configured",
		},

		// ── Catálogo vazio / pré-requisitos ──
		"tui_no_view_available": {
			PT: "nenhuma view disponível no catálogo. Crie uma view primeiro",
			ES: "ninguna view disponible en el catálogo. Cree una view primero",
			EN: "no view available in the catalog. Create a view first",
		},
		"tui_no_table_available": {
			PT: "nenhuma tabela disponível no catálogo. Crie uma tabela usando CreateTable primeiro",
			ES: "ninguna tabla disponible en el catálogo. Cree una tabla usando CreateTable primero",
			EN: "no table available in the catalog. Create a table using CreateTable first",
		},
		"tui_no_pk_table": {
			PT: "  Nenhuma tabela com chave primária declarada.\n\n  [Enter] Voltar\n",
			ES: "  Ninguna tabla con clave primaria declarada.\n\n  [Enter] Volver\n",
			EN: "  No table with a declared primary key.\n\n  [Enter] Back\n",
		},
		"tui_no_factory": {
			PT: "  Nenhuma factory encontrada.\n\n  [Enter] Voltar\n",
			ES: "  Ninguna factory encontrada.\n\n  [Enter] Volver\n",
			EN: "  No factory found.\n\n  [Enter] Back\n",
		},

		// ── Criação de migration ──
		"tui_selected_table": {
			PT: "  Tabela selecionada: %s\n\n",
			ES: "  Tabla seleccionada: %s\n\n",
			EN: "  Selected table: %s\n\n",
		},
		"tui_prompt_field_name": {
			PT: "  Digite o nome descritivo do campo/constraint (ex: email ou chk_users_age):\n",
			ES: "  Escriba el nombre descriptivo del campo/constraint (ej: email o chk_users_age):\n",
			EN: "  Type the descriptive name of the field/constraint (e.g. email or chk_users_age):\n",
		},
		"tui_prompt_struct_name": {
			PT: "  Digite o nome descritivo da nova estrutura (ex: users ou active_users):\n",
			ES: "  Escriba el nombre descriptivo de la nueva estructura (ej: users o active_users):\n",
			EN: "  Type the descriptive name of the new structure (e.g. users or active_users):\n",
		},
		"tui_mig_create_error": {
			PT: "%s Erro ao criar migration:\n\n%s\n\n",
			ES: "%s Error al crear la migración:\n\n%s\n\n",
			EN: "%s Failed to create the migration:\n\n%s\n\n",
		},
		"tui_mig_creating": {
			PT: "%s Criando nova estrutura de migration...\n\n%s\n\n",
			ES: "%s Creando nueva estructura de migración...\n\n%s\n\n",
			EN: "%s Creating the new migration structure...\n\n%s\n\n",
		},
		"tui_mig_created": {
			PT: "✔ Migration criada com sucesso: %s",
			ES: "✔ Migración creada con éxito: %s",
			EN: "✔ Migration created successfully: %s",
		},

		// ── Reload em andamento ──
		"tui_reload_running": {
			PT: " Reload em andamento...",
			ES: " Reload en curso...",
			EN: " Reload in progress...",
		},
		"tui_reload_wait": {
			PT: "  Preparando ambiente, banco, migrations e catálogos. Aguarde.",
			ES: "  Preparando ambiente, base, migraciones y catálogos. Espere.",
			EN: "  Preparing environment, database, migrations and catalogs. Please wait.",
		},

		// ── Doctor · checklist ──
		"tui_doctor_checklist": {
			PT: "🔍 Doctor · checklist",
			ES: "🔍 Doctor · checklist",
			EN: "🔍 Doctor · checklist",
		},
		"tui_cfg_loaded": {
			PT: "gokit.json carregado",
			ES: "gokit.json cargado",
			EN: "gokit.json loaded",
		},
		"tui_cfg_invalid": {
			PT: "gokit.json inválido: %v",
			ES: "gokit.json inválido: %v",
			EN: "invalid gokit.json: %v",
		},
		"tui_env_duplicates": {
			PT: "variáveis duplicadas: ",
			ES: "variables duplicadas: ",
			EN: "duplicate variables: ",
		},
		"tui_conn_failed": {
			PT: "falha na conexão: %v",
			ES: "fallo en la conexión: %v",
			EN: "connection failed: %v",
		},
		"tui_version_ok": {
			PT: "versão compatível: ",
			ES: "versión compatible: ",
			EN: "compatible version: ",
		},
		"tui_version_bad": {
			PT: "versão incompatível: ",
			ES: "versión incompatible: ",
			EN: "incompatible version: ",
		},
		"tui_ddl_ok": {
			PT: "permissões CREATE/DROP disponíveis",
			ES: "permisos CREATE/DROP disponibles",
			EN: "CREATE/DROP permissions available",
		},
		"tui_ddl_fail": {
			PT: "permissões DDL falharam: %v",
			ES: "los permisos DDL fallaron: %v",
			EN: "DDL permissions failed: %v",
		},
		"tui_no_conn_skipped": {
			PT: "⚠️ versão e permissões não verificadas sem conexão",
			ES: "⚠️ versión y permisos no verificados sin conexión",
			EN: "⚠️ version and permissions not checked without a connection",
		},
		"tui_update_check_failed": {
			PT: "⚠️ não foi possível consultar atualizações no Git",
			ES: "⚠️ no se pudo consultar actualizaciones en Git",
			EN: "⚠️ could not check for updates on Git",
		},
		"tui_update_exec_available": {
			PT: "⚠️ atualização do exec disponível: ",
			ES: "⚠️ actualización del ejecutable disponible: ",
			EN: "⚠️ executable update available: ",
		},
		"tui_exec_current": {
			PT: "✅ exec atualizado",
			ES: "✅ ejecutable actualizado",
			EN: "✅ executable up to date",
		},
	})
}
