package i18n

// Mensagens do Reload: os três grupos de etapas (ambiente, execução, geração),
// o rótulo de cada passo e o par erro/sugestão que cada verificação devolve.
//
// Os códigos \033[..m são cor ANSI e atravessam a tradução intactos — só o texto
// entre eles muda de idioma.
func init() {
	Register(Entradas{
		// ── Grupos e desfecho ──
		"rel_group_env": {
			PT: "\nAmbiente & infraestrutura",
			ES: "\nAmbiente e infraestructura",
			EN: "\nEnvironment & infrastructure",
		},
		"rel_group_exec": {
			PT: "\nOperações & execução",
			ES: "\nOperaciones y ejecución",
			EN: "\nOperations & execution",
		},
		"rel_group_gen": {
			PT: "\nGeração & catálogos",
			ES: "\nGeneración y catálogos",
			EN: "\nGeneration & catalogs",
		},
		"rel_error_line": {
			PT: "\n  \033[31mErro:\033[0m %s\n",
			ES: "\n  \033[31mError:\033[0m %s\n",
			EN: "\n  \033[31mError:\033[0m %s\n",
		},
		"rel_hint_line": {
			PT: "  \033[36mSugestão:\033[0m %s\n\n",
			ES: "  \033[36mSugerencia:\033[0m %s\n\n",
			EN: "  \033[36mHint:\033[0m %s\n\n",
		},
		"rel_aborted_g1": {
			PT: "reload abortado no Grupo 1: %w",
			ES: "reload abortado en el Grupo 1: %w",
			EN: "reload aborted in Group 1: %w",
		},
		"rel_aborted_g2": {
			PT: "reload abortado no Grupo 2: %w",
			ES: "reload abortado en el Grupo 2: %w",
			EN: "reload aborted in Group 2: %w",
		},
		"rel_aborted_g3": {
			PT: "reload abortado no Grupo 3: %w",
			ES: "reload abortado en el Grupo 3: %w",
			EN: "reload aborted in Group 3: %w",
		},
		"rel_success": {
			PT: "\n\033[32m✔ Projeto redefinido e alinhado com sucesso!\033[0m",
			ES: "\n\033[32m✔ ¡Proyecto redefinido y alineado con éxito!\033[0m",
			EN: "\n\033[32m✔ Project reset and aligned successfully!\033[0m",
		},

		// ── Rótulos dos passos ──
		"rel_step_docker": {
			PT: "Verificar estado e daemon do Docker",
			ES: "Verificar el estado y el daemon de Docker",
			EN: "Check Docker state and daemon",
		},
		"rel_step_toolchain": {
			PT: "Verificar o toolchain Golang configurado",
			ES: "Verificar el toolchain Golang configurado",
			EN: "Check the configured Go toolchain",
		},
		"rel_step_config": {
			PT: "Validar gokit.json e .env",
			ES: "Validar gokit.json y .env",
			EN: "Validate gokit.json and .env",
		},
		"rel_step_dirs": {
			PT: "Alinhar diretórios físicos com o gokit.json",
			ES: "Alinear directorios físicos con el gokit.json",
			EN: "Align physical directories with gokit.json",
		},
		"rel_step_names": {
			PT: "Normalizar nomes dinâmicos de migrations e seeders",
			ES: "Normalizar nombres dinámicos de migraciones y seeders",
			EN: "Normalize dynamic migration and seeder names",
		},
		"rel_step_tidy": {
			PT: "Atualizar dependências (go mod tidy)",
			ES: "Actualizar dependencias (go mod tidy)",
			EN: "Update dependencies (go mod tidy)",
		},
		"rel_step_compose": {
			PT: "Iniciar o banco ativo pelo Docker Compose",
			ES: "Iniciar la base activa con Docker Compose",
			EN: "Start the active database via Docker Compose",
		},
		"rel_step_ping": {
			PT: "Ping físico com o Banco de Dados",
			ES: "Ping físico con la Base de Datos",
			EN: "Physical ping to the database",
		},
		"rel_step_migrations": {
			PT: "Verificar e executar migrations pendentes",
			ES: "Verificar y ejecutar migraciones pendientes",
			EN: "Check and run pending migrations",
		},
		"rel_step_seeders": {
			PT: "Verificar e executar seeders pendentes",
			ES: "Verificar y ejecutar seeders pendientes",
			EN: "Check and run pending seeders",
		},
		"rel_step_factories": {
			PT: "Verificar e rodar factories ativas",
			ES: "Verificar y ejecutar factories activas",
			EN: "Check and run active factories",
		},
		"rel_step_orm": {
			PT: "Gerar mapeamentos ORM a partir das migrations",
			ES: "Generar mapeos ORM a partir de las migraciones",
			EN: "Generate ORM mappings from the migrations",
		},
		"rel_step_dsl": {
			PT: "Reconstruir catálogo de autocomplete do Core (dsl.gen.go)",
			ES: "Reconstruir el catálogo de autocompletado del Core (dsl.gen.go)",
			EN: "Rebuild the Core autocomplete catalog (dsl.gen.go)",
		},
		"rel_step_docs": {
			PT: "Gerar documentação automática do banco e das migrations",
			ES: "Generar documentación automática de la base y de las migraciones",
			EN: "Generate automatic documentation of the database and migrations",
		},

		// ── Toolchain e configuração ──
		"rel_names_normalized": {
			PT: "%d declaração(ões) normalizada(s)",
			ES: "%d declaración(es) normalizada(s)",
			EN: "%d declaration(s) normalized",
		},
		"rel_toolchain_failed": {
			PT: "Toolchain Go não pôde ser executado: %s",
			ES: "El toolchain Go no pudo ejecutarse: %s",
			EN: "The Go toolchain could not be run: %s",
		},
		"rel_toolchain_failed_fix": {
			PT: "Verifique go.execution e go.docker_service no gokit.json ou instale o Go no PATH para execução host.",
			ES: "Revise go.execution y go.docker_service en el gokit.json o instale Go en el PATH para ejecución host.",
			EN: "Check go.execution and go.docker_service in gokit.json, or install Go on the PATH for host execution.",
		},
		"rel_config_invalid": {
			PT: "Arquivo gokit.json ou .env inválido: %v",
			ES: "Archivo gokit.json o .env inválido: %v",
			EN: "Invalid gokit.json or .env file: %v",
		},
		"rel_config_invalid_fix": {
			PT: "Verifique o formato JSON do internal/gokit/gokit.json ou remova-o para gerar o scaffold de onboarding.",
			ES: "Revise el formato JSON de internal/gokit/gokit.json o elimínelo para generar el scaffold de onboarding.",
			EN: "Check the JSON format of internal/gokit/gokit.json, or delete it to generate the onboarding scaffold.",
		},
		"rel_config_empty": {
			PT: "Configuração do GoKit vazia ou nula.",
			ES: "Configuración de GoKit vacía o nula.",
			EN: "GoKit configuration is empty or nil.",
		},
		"rel_config_empty_fix": {
			PT: "Execute o GoKit na pasta para criar o gokit.json.",
			ES: "Ejecute GoKit en la carpeta para crear el gokit.json.",
			EN: "Run GoKit in the folder to create gokit.json.",
		},
		"rel_config_ok": {
			PT: "gokit.json e .env válidos.",
			ES: "gokit.json y .env válidos.",
			EN: "gokit.json and .env are valid.",
		},

		// ── Docker Compose e banco ──
		"rel_autostart_off": {
			PT: "Inicialização automática desativada no gokit.json.",
			ES: "Inicio automático desactivado en el gokit.json.",
			EN: "Automatic startup is disabled in gokit.json.",
		},
		"rel_db_already_up": {
			PT: "Banco já estava em execução.",
			ES: "La base ya estaba en ejecución.",
			EN: "The database was already running.",
		},
		"rel_compose_missing": {
			PT: "Arquivo Docker Compose não encontrado na raiz do projeto.",
			ES: "Archivo Docker Compose no encontrado en la raíz del proyecto.",
			EN: "Docker Compose file not found at the project root.",
		},
		"rel_compose_missing_fix": {
			PT: "Crie o serviço do banco ou defina go.docker_auto_start como false para usar um banco externo.",
			ES: "Cree el servicio de la base o defina go.docker_auto_start como false para usar una base externa.",
			EN: "Create the database service, or set go.docker_auto_start to false to use an external database.",
		},
		"rel_compose_missing_err": {
			PT: "docker compose ausente",
			ES: "docker compose ausente",
			EN: "docker compose missing",
		},
		"rel_no_service_for_dialect": {
			PT: "Não há serviço Docker conhecido para o dialeto %q.",
			ES: "No hay servicio Docker conocido para el dialecto %q.",
			EN: "There is no known Docker service for dialect %q.",
		},
		"rel_no_service_for_dialect_err": {
			PT: "dialeto sem servico docker: %s",
			ES: "dialecto sin servicio docker: %s",
			EN: "dialect without a docker service: %s",
		},
		"rel_service_inactive": {
			PT: "O serviço %q não está ativo no %s.",
			ES: "El servicio %q no está activo en %s.",
			EN: "Service %q is not active in %s.",
		},
		"rel_service_inactive_fix": {
			PT: "Descomente ou adicione o serviço %q no Docker Compose para o dialeto %s.",
			ES: "Descomente o agregue el servicio %q en el Docker Compose para el dialecto %s.",
			EN: "Uncomment or add service %q in the Docker Compose for dialect %s.",
		},
		"rel_service_inactive_err": {
			PT: "servico docker %s ausente",
			ES: "servicio docker %s ausente",
			EN: "docker service %s missing",
		},
		"rel_service_start_failed": {
			PT: "Falha ao iniciar o serviço %s: %s",
			ES: "Fallo al iniciar el servicio %s: %s",
			EN: "Failed to start service %s: %s",
		},
		"rel_service_start_failed_fix": {
			PT: "Execute 'docker compose up -d %s' para inspecionar o erro.",
			ES: "Ejecute 'docker compose up -d %s' para inspeccionar el error.",
			EN: "Run 'docker compose up -d %s' to inspect the error.",
		},
		"rel_service_ready": {
			PT: "Serviço %s iniciado e banco pronto.",
			ES: "Servicio %s iniciado y base lista.",
			EN: "Service %s started and database ready.",
		},
		"rel_service_not_ready": {
			PT: "O serviço %s iniciou, mas o banco não ficou pronto: %v",
			ES: "El servicio %s inició, pero la base no quedó lista: %v",
			EN: "Service %s started, but the database never became ready: %v",
		},
		"rel_service_not_ready_fix": {
			PT: "Inspecione os logs com 'docker compose logs %s'.",
			ES: "Inspeccione los logs con 'docker compose logs %s'.",
			EN: "Inspect the logs with 'docker compose logs %s'.",
		},
		"rel_no_active_conn": {
			PT: "Sem conexão ativa configurada.",
			ES: "Sin conexión activa configurada.",
			EN: "No active connection configured.",
		},
		"rel_no_active_conn_fix": {
			PT: "Verifique se a variável DB_DIALECT no .env está preenchida corretamente.",
			ES: "Verifique que la variable DB_DIALECT en el .env esté correctamente definida.",
			EN: "Check that the DB_DIALECT variable in .env is filled in correctly.",
		},
		"rel_conn_failed": {
			PT: "Falha ao se conectar ao banco %s: %v",
			ES: "Fallo al conectarse a la base %s: %v",
			EN: "Failed to connect to database %s: %v",
		},
		"rel_conn_failed_fix": {
			PT: "Certifique-se de que o container do banco de dados está rodando e a porta está correta no .env.",
			ES: "Asegúrese de que el contenedor de la base esté corriendo y que el puerto en el .env sea el correcto.",
			EN: "Make sure the database container is running and the port in .env is correct.",
		},
		"rel_ping_ok": {
			PT: "Banco de dados respondendo ao Ping.",
			ES: "La base de datos responde al Ping.",
			EN: "The database is answering the ping.",
		},

		// ── Daemon do Docker ──
		"rel_docker_cli_missing": {
			PT: "CLI do Docker não foi localizado no PATH do sistema.",
			ES: "El CLI de Docker no fue localizado en el PATH del sistema.",
			EN: "The Docker CLI was not found on the system PATH.",
		},
		"rel_docker_cli_missing_fix": {
			PT: "Instale o Docker Desktop em https://www.docker.com/products/docker-desktop/ ou ative a CLI do Docker no seu ambiente.",
			ES: "Instale Docker Desktop en https://www.docker.com/products/docker-desktop/ o active el CLI de Docker en su ambiente.",
			EN: "Install Docker Desktop from https://www.docker.com/products/docker-desktop/ or enable the Docker CLI in your environment.",
		},
		"rel_docker_ok": {
			PT: "Daemon Docker ativo e comunicando.",
			ES: "Daemon Docker activo y comunicando.",
			EN: "Docker daemon is up and responding.",
		},
		"rel_docker_autostarted": {
			PT: "Daemon Docker estava inativo, mas foi iniciado automaticamente com sucesso.",
			ES: "El daemon Docker estaba inactivo, pero fue iniciado automáticamente con éxito.",
			EN: "The Docker daemon was down, but it was started automatically.",
		},
		"rel_docker_offline": {
			PT: "Docker Daemon inativo (não foi possível estabelecer comunicação com o socket do Docker).",
			ES: "Daemon Docker inactivo (no se pudo establecer comunicación con el socket de Docker).",
			EN: "Docker daemon is down (could not reach the Docker socket).",
		},
		"rel_docker_offline_fix": {
			PT: "Inicie o aplicativo Docker Desktop ou o serviço do Docker (ex: 'colima start' ou 'systemctl start docker').",
			ES: "Inicie la aplicación Docker Desktop o el servicio de Docker (ej: 'colima start' o 'systemctl start docker').",
			EN: "Start the Docker Desktop app or the Docker service (e.g. 'colima start' or 'systemctl start docker').",
		},
		"rel_docker_offline_err": {
			PT: "docker daemon offline",
			ES: "docker daemon offline",
			EN: "docker daemon offline",
		},

		// ── go.mod / tidy ──
		"rel_gomod_failed": {
			PT: "Não foi possível alinhar o go.mod: %v",
			ES: "No se pudo alinear el go.mod: %v",
			EN: "Could not align go.mod: %v",
		},
		"rel_gomod_failed_fix": {
			PT: "Revise a seção go do internal/gokit/gokit.json, especialmente module e gokit_local.",
			ES: "Revise la sección go de internal/gokit/gokit.json, especialmente module y gokit_local.",
			EN: "Review the go section of internal/gokit/gokit.json, particularly module and gokit_local.",
		},
		"rel_tidy_failed": {
			PT: "Falha ao executar 'go mod tidy': %s",
			ES: "Fallo al ejecutar 'go mod tidy': %s",
			EN: "Failed to run 'go mod tidy': %s",
		},
		"rel_tidy_failed_fix": {
			PT: "Verifique a sintaxe dos arquivos .go ou remova caminhos de 'replace' inválidos no seu go.mod.",
			ES: "Revise la sintaxis de los archivos .go o elimine rutas de 'replace' inválidas en su go.mod.",
			EN: "Check the syntax of the .go files, or remove invalid 'replace' paths from your go.mod.",
		},
		"rel_module_aligned_mode": {
			PT: "Módulo %s alinhado com %s (modo %s) via %s.",
			ES: "Módulo %s alineado con %s (modo %s) vía %s.",
			EN: "Module %s aligned with %s (%s mode) via %s.",
		},
		"rel_module_aligned": {
			PT: "Módulo %s alinhado com %s via %s.",
			ES: "Módulo %s alineado con %s vía %s.",
			EN: "Module %s aligned with %s via %s.",
		},

		// ── Migrations, seeders, factories e geração ──
		"rel_load_migrations_failed": {
			PT: "Erro ao carregar migrations locais: %v",
			ES: "Error al cargar migraciones locales: %v",
			EN: "Failed to load local migrations: %v",
		},
		"rel_open_db_failed": {
			PT: "Erro ao abrir banco: %v",
			ES: "Error al abrir la base: %v",
			EN: "Failed to open the database: %v",
		},
		"rel_migrations_pending": {
			PT: "%d migration(s) pendente(s) não foram aplicadas; veja o diagnóstico e as soluções abaixo.",
			ES: "%d migración(es) pendiente(s) no fueron aplicadas; vea el diagnóstico y las soluciones abajo.",
			EN: "%d pending migration(s) were not applied; see the diagnosis and fixes below.",
		},
		"rel_applied_n": {
			PT: "%d aplicadas",
			ES: "%d aplicadas",
			EN: "%d applied",
		},
		"rel_none_pending_f": {
			PT: "Nenhuma pendente",
			ES: "Ninguna pendiente",
			EN: "None pending",
		},
		"rel_no_seeder_pending": {
			PT: "Nenhum seeder pendente",
			ES: "Ningún seeder pendiente",
			EN: "No seeder pending",
		},
		"rel_seeders_failed": {
			PT: "Erro ao rodar seeders: %v",
			ES: "Error al ejecutar seeders: %v",
			EN: "Failed to run seeders: %v",
		},
		"rel_applied_n_m": {
			PT: "%d aplicados",
			ES: "%d aplicados",
			EN: "%d applied",
		},
		"rel_none_pending_m": {
			PT: "Nenhum pendente",
			ES: "Ninguno pendiente",
			EN: "None pending",
		},
		"rel_none_active": {
			PT: "Nenhuma ativa",
			ES: "Ninguna activa",
			EN: "None active",
		},
		"rel_factories_failed": {
			PT: "Falha ao popular factories: %v",
			ES: "Fallo al poblar factories: %v",
			EN: "Failed to populate factories: %v",
		},
		"rel_tables_populated": {
			PT: "%d tabelas populadas",
			ES: "%d tablas pobladas",
			EN: "%d tables populated",
		},
		"rel_orm_failed": {
			PT: "Falha ao gerar ORM: %v",
			ES: "Fallo al generar el ORM: %v",
			EN: "Failed to generate the ORM: %v",
		},
		"rel_orm_done": {
			PT: "%d entidade(s) mapeada(s) em core.gen.go.",
			ES: "%d entidad(es) mapeada(s) en core.gen.go.",
			EN: "%d entity(ies) mapped in core.gen.go.",
		},
		"rel_catalog_failed": {
			PT: "Falha ao reconstruir catálogo: %v",
			ES: "Fallo al reconstruir el catálogo: %v",
			EN: "Failed to rebuild the catalog: %v",
		},
		"rel_catalog_done": {
			PT: "Autocompletes do Core gerados.",
			ES: "Autocompletados del Core generados.",
			EN: "Core autocompletes generated.",
		},
		"rel_docs_failed": {
			PT: "Falha ao gerar documentação: %v",
			ES: "Fallo al generar la documentación: %v",
			EN: "Failed to generate the documentation: %v",
		},
		"rel_docs_done": {
			PT: "database.md e migrations.md atualizados.",
			ES: "database.md y migrations.md actualizados.",
			EN: "database.md and migrations.md updated.",
		},
		"rel_dir_move_failed": {
			PT: "Erro ao migrar pasta %s para %s: %v",
			ES: "Error al migrar la carpeta %s a %s: %v",
			EN: "Failed to move folder %s to %s: %v",
		},
		"rel_dirs_aligned_n": {
			PT: "%d diretórios alinhados",
			ES: "%d directorios alineados",
			EN: "%d directories aligned",
		},
		"rel_dirs_already": {
			PT: "Diretórios já alinhados",
			ES: "Directorios ya alineados",
			EN: "Directories already aligned",
		},
	})
}
