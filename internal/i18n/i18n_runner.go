package i18n

// Mensagens de execução do runner de migrations: validação do corpus, aplicação,
// rollback, seed e os conselhos de conexão. São as frases que o usuário lê no
// terminal quando algo dá certo — ou, mais importante, quando dá errado.
//
// Frases de erro usam %w para embrulhar a causa; quem chama passa por
// i18n.Errf, então errors.Is/errors.As continuam atravessando a tradução.
func init() {
	Register(Entradas{
		// ── Validação do corpus ──
		"run_changed_after_run": {
			PT: "migration %s foi alterada depois de executada",
			ES: "la migración %s fue alterada después de ejecutada",
			EN: "migration %s was changed after being run",
		},
		"run_load_problems": {
			PT: "%d migration(s) com problema:",
			ES: "%d migración(es) con problema:",
			EN: "%d migration(s) with problems:",
		},
		"run_problems_found": {
			PT: "%s %d problema(s) encontrado(s):\n",
			ES: "%s %d problema(s) encontrado(s):\n",
			EN: "%s %d problem(s) found:\n",
		},
		"run_by_cause": {
			PT: "\nPor causa:",
			ES: "\nPor causa:",
			EN: "\nBy cause:",
		},
		"run_by_file": {
			PT: "\nPor arquivo:",
			ES: "\nPor archivo:",
			EN: "\nBy file:",
		},
		"run_prevalidation_count": {
			PT: "%d migration(s) não passaram na pré-validação.",
			ES: "%d migración(es) no pasaron la prevalidación.",
			EN: "%d migration(s) failed pre-validation.",
		},
		"run_prevalidation_fix": {
			PT: "Corrija os arquivos listados acima e rode gokit migrate validate novamente.",
			ES: "Corrija los archivos listados arriba y ejecute gokit migrate validate de nuevo.",
			EN: "Fix the files listed above and run gokit migrate validate again.",
		},
		"run_prevalidation_list": {
			PT: "%s %d migration(s) não passaram na pré-validação:\n%s\n",
			ES: "%s %d migración(es) no pasaron la prevalidación:\n%s\n",
			EN: "%s %d migration(s) failed pre-validation:\n%s\n",
		},
		"run_corpus_broken": {
			PT: "O corpus de migrations tem erros e nada foi aplicado.",
			ES: "El corpus de migraciones tiene errores y nada fue aplicado.",
			EN: "The migration corpus has errors and nothing was applied.",
		},
		"run_corpus_broken_fix": {
			PT: "Rode gokit migrate validate para ver arquivo por arquivo.",
			ES: "Ejecute gokit migrate validate para ver archivo por archivo.",
			EN: "Run gokit migrate validate to inspect file by file.",
		},
		"run_ok_files": {
			PT: "%s %d migration(s), %d operação(ões)\n",
			ES: "%s %d migración(es), %d operación(es)\n",
			EN: "%s %d migration(s), %d operation(s)\n",
		},

		// ── Aplicação ──
		"run_none_generated": {
			PT: "Nenhuma migration foi gerada.",
			ES: "Ninguna migración fue generada.",
			EN: "No migration has been generated.",
		},
		"run_create_first": {
			PT: "Crie um arquivo de migration em Go sob %s para começar.",
			ES: "Cree un archivo de migración en Go bajo %s para empezar.",
			EN: "Create a Go migration file under %s to get started.",
		},
		"run_diagnosis_label": {
			PT: "⚠ Diagnóstico:",
			ES: "⚠ Diagnóstico:",
			EN: "⚠ Diagnosis:",
		},
		"run_history_diverged": {
			PT: "O histórico de migrations divergiu dos arquivos locais.",
			ES: "El historial de migraciones divergió de los archivos locales.",
			EN: "The migration history diverged from the local files.",
		},
		"run_conn_incomplete": {
			PT: "A conexão ativa não concluiu as migrations.",
			ES: "La conexión activa no concluyó las migraciones.",
			EN: "The active connection did not finish the migrations.",
		},
		"run_applied_count": {
			PT: "  %s %d aplicada(s), %d já executada(s)\n",
			ES: "  %s %d aplicada(s), %d ya ejecutada(s)\n",
			EN: "  %s %d applied, %d already run\n",
		},
		"run_docs_failed": {
			PT: "As migrations foram aplicadas, mas a documentação não pôde ser atualizada.",
			ES: "Las migraciones fueron aplicadas, pero la documentación no pudo actualizarse.",
			EN: "The migrations were applied, but the documentation could not be updated.",
		},
		"run_migrations_updated": {
			PT: "✓ Migrations atualizadas na conexão padrão.",
			ES: "✓ Migraciones actualizadas en la conexión predeterminada.",
			EN: "✓ Migrations updated on the default connection.",
		},
		"run_dialect_unsupported": {
			PT: "dialeto não suportado: %s",
			ES: "dialecto no soportado: %s",
			EN: "unsupported dialect: %s",
		},

		// ── Rollback ──
		"run_none_found": {
			PT: "Nenhuma migration encontrada.",
			ES: "Ninguna migración encontrada.",
			EN: "No migration found.",
		},
		"run_none_available": {
			PT: "Nenhuma migration disponível.",
			ES: "Ninguna migración disponible.",
			EN: "No migration available.",
		},
		"run_batch_fetch_failed": {
			PT: "erro ao buscar último batch: %w",
			ES: "error al buscar el último batch: %w",
			EN: "failed to fetch the last batch: %w",
		},
		"run_nothing_to_rollback": {
			PT: "Nenhuma migration para reverter.",
			ES: "Ninguna migración para revertir.",
			EN: "No migration to roll back.",
		},
		"run_history_fetch_failed": {
			PT: "erro ao buscar histórico do lote %d: %w",
			ES: "error al buscar el historial del lote %d: %w",
			EN: "failed to fetch the history of batch %d: %w",
		},
		"run_batch_empty": {
			PT: "Nenhuma migration encontrada para reverter no lote",
			ES: "Ninguna migración encontrada para revertir en el lote",
			EN: "No migration found to roll back in batch",
		},
		"run_reverting_batch": {
			PT: "Revertendo lote %d (%d migrations)...\n",
			ES: "Revirtiendo lote %d (%d migraciones)...\n",
			EN: "Rolling back batch %d (%d migrations)...\n",
		},
		"run_file_missing": {
			PT: "arquivo da migration %s não encontrado no projeto. Impossível reverter sem a definição Go",
			ES: "archivo de la migración %s no encontrado en el proyecto. Imposible revertir sin la definición Go",
			EN: "migration file %s not found in the project. Cannot roll back without the Go definition",
		},
		"run_revert_failed": {
			PT: "falha ao reverter operação %s na tabela %s: %w",
			ES: "fallo al revertir la operación %s en la tabla %s: %w",
			EN: "failed to revert operation %s on table %s: %w",
		},
		"run_history_delete_failed": {
			PT: "erro ao remover histórico da migration %s: %w",
			ES: "error al eliminar el historial de la migración %s: %w",
			EN: "failed to remove the history of migration %s: %w",
		},
		"run_rollback_done": {
			PT: "✓ Rollback concluído com sucesso.",
			ES: "✓ Rollback concluido con éxito.",
			EN: "✓ Rollback completed successfully.",
		},
		"run_not_reversible": {
			PT: "a operação %s não é reversível automaticamente; reverta manualmente ou crie uma migration de correção",
			ES: "la operación %s no es reversible automáticamente; revierta manualmente o cree una migración de corrección",
			EN: "operation %s is not automatically reversible; revert it manually or create a corrective migration",
		},
		"run_drop_not_recreatable": {
			PT: "a operação %s removeu um objeto e não pode ser recriada automaticamente; reverta manualmente",
			ES: "la operación %s eliminó un objeto y no puede recrearse automáticamente; revierta manualmente",
			EN: "operation %s dropped an object and cannot be recreated automatically; revert it manually",
		},
		"run_alter_no_previous": {
			PT: "alter_column não guarda a definição anterior da coluna; reverta manualmente",
			ES: "alter_column no guarda la definición anterior de la columna; revierta manualmente",
			EN: "alter_column does not keep the column's previous definition; revert it manually",
		},
		"run_no_rollback_defined": {
			PT: "operação %s não tem rollback definido",
			ES: "la operación %s no tiene rollback definido",
			EN: "operation %s has no rollback defined",
		},
		"run_view_removed": {
			PT: "  - view %s removida\n",
			ES: "  - view %s eliminada\n",
			EN: "  - view %s removed\n",
		},
		"run_table_removed": {
			PT: "  - tabela %s removida\n",
			ES: "  - tabla %s eliminada\n",
			EN: "  - table %s removed\n",
		},
		"run_history_drop_failed": {
			PT: "remover histórico %s: %w",
			ES: "eliminar el historial %s: %w",
			EN: "removing history %s: %w",
		},

		// ── Conselhos de conexão (o diagnóstico que acompanha a falha) ──
		"run_advice_changed": {
			PT: "%s já foi executada e o arquivo local agora possui outro conteúdo; a conexão com o banco está funcionando",
			ES: "%s ya fue ejecutada y el archivo local ahora tiene otro contenido; la conexión con la base funciona",
			EN: "%s has already run and the local file now has different content; the database connection is working",
		},
		"run_advice_changed_fix": {
			PT: "Não altere nem apague uma migration já executada.\nSe ela foi compartilhada: restaure o arquivo original e mantenha a correção ou o Drop em uma nova migration.\nSe o banco é descartável e somente de desenvolvimento: restaure o arquivo ou remova apenas os arquivos locais ainda não executados e use Reload Fresh para reconstruir o banco.\nSe Create e Drop nunca foram executados em nenhum ambiente: remova os dois arquivos e rode Reload novamente.",
			ES: "No altere ni borre una migración ya ejecutada.\nSi fue compartida: restaure el archivo original y deje la corrección o el Drop en una nueva migración.\nSi la base es descartable y solo de desarrollo: restaure el archivo o elimine solo los archivos locales aún no ejecutados y use Reload Fresh para reconstruir la base.\nSi Create y Drop nunca se ejecutaron en ningún ambiente: elimine los dos archivos y ejecute Reload de nuevo.",
			EN: "Never change or delete a migration that has already run.\nIf it was shared: restore the original file and put the fix or the Drop in a new migration.\nIf the database is disposable and development-only: restore the file, or remove only the local files not yet run and use Reload Fresh to rebuild the database.\nIf Create and Drop never ran in any environment: remove both files and run Reload again.",
		},
		"run_advice_local_refused": {
			PT: "%s é uma conexão local; este erro não depende da internet e indica que a porta, o container ou o listener não aceitou a sessão",
			ES: "%s es una conexión local; este error no depende de internet e indica que el puerto, el contenedor o el listener no aceptó la sesión",
			EN: "%s is a local connection; this error does not depend on the internet and means the port, the container or the listener refused the session",
		},
		"run_advice_local_refused_fix": {
			PT: "Confirme o container, a porta publicada e o listener Oracle; depois execute Connection e Migrate Run novamente.",
			ES: "Confirme el contenedor, el puerto publicado y el listener Oracle; luego ejecute Connection y Migrate Run de nuevo.",
			EN: "Check the container, the published port and the Oracle listener; then run Connection and Migrate Run again.",
		},
		"run_advice_remote_refused": {
			PT: "%s é um host remoto; o servidor recusou a conexão e pode haver indisponibilidade, firewall, VPN ou falha de internet",
			ES: "%s es un host remoto; el servidor rechazó la conexión y puede haber indisponibilidad, firewall, VPN o fallo de internet",
			EN: "%s is a remote host; the server refused the connection — it may be down, or blocked by a firewall, VPN or internet failure",
		},
		"run_advice_remote_refused_fix": {
			PT: "Confira sua internet/VPN, DNS, firewall, host e porta; depois execute Connection e Migrate Run novamente.",
			ES: "Revise su internet/VPN, DNS, firewall, host y puerto; luego ejecute Connection y Migrate Run de nuevo.",
			EN: "Check your internet/VPN, DNS, firewall, host and port; then run Connection and Migrate Run again.",
		},
		"run_advice_dns": {
			PT: "não foi possível resolver o host %s; a causa pode ser DNS, VPN ou conexão com a internet",
			ES: "no se pudo resolver el host %s; la causa puede ser DNS, VPN o la conexión a internet",
			EN: "could not resolve host %s; the cause may be DNS, VPN or the internet connection",
		},
		"run_advice_dns_fix": {
			PT: "Confira sua internet/VPN e o nome do host no connection.yaml; depois execute Connection novamente.",
			ES: "Revise su internet/VPN y el nombre del host en connection.yaml; luego ejecute Connection de nuevo.",
			EN: "Check your internet/VPN and the host name in connection.yaml; then run Connection again.",
		},
		"run_advice_local_timeout": {
			PT: "%s é local; o serviço não respondeu no tempo esperado e a internet não é necessária",
			ES: "%s es local; el servicio no respondió en el tiempo esperado y la internet no es necesaria",
			EN: "%s is local; the service did not answer in time and the internet is not involved",
		},
		"run_advice_local_timeout_fix": {
			PT: "Confira a saúde do container, a porta e os logs do banco; depois execute Connection novamente.",
			ES: "Revise la salud del contenedor, el puerto y los logs de la base; luego ejecute Connection de nuevo.",
			EN: "Check the container health, the port and the database logs; then run Connection again.",
		},
		"run_advice_remote_timeout": {
			PT: "%s não respondeu; verifique internet, VPN, rota, firewall e disponibilidade do servidor",
			ES: "%s no respondió; verifique internet, VPN, ruta, firewall y disponibilidad del servidor",
			EN: "%s did not answer; check internet, VPN, routing, firewall and server availability",
		},
		"run_advice_remote_timeout_fix": {
			PT: "Teste sua internet/VPN e a conectividade com o host e a porta antes de repetir Migrate Run.",
			ES: "Pruebe su internet/VPN y la conectividad con el host y el puerto antes de repetir Migrate Run.",
			EN: "Test your internet/VPN and the connectivity to the host and port before repeating Migrate Run.",
		},
		"run_advice_oracle_service": {
			PT: "o listener respondeu, mas o serviço Oracle configurado não foi encontrado",
			ES: "el listener respondió, pero el servicio Oracle configurado no fue encontrado",
			EN: "the listener answered, but the configured Oracle service was not found",
		},
		"run_advice_oracle_service_fix": {
			PT: "Confira ORACLE_SERVICE no .env e os serviços registrados no listener; depois execute Connection novamente.",
			ES: "Revise ORACLE_SERVICE en el .env y los servicios registrados en el listener; luego ejecute Connection de nuevo.",
			EN: "Check ORACLE_SERVICE in .env and the services registered in the listener; then run Connection again.",
		},
		"run_advice_db_refused": {
			PT: "a conexão foi alcançada, mas o banco recusou ou interrompeu a operação",
			ES: "la conexión fue alcanzada, pero la base rechazó o interrumpió la operación",
			EN: "the connection was reached, but the database refused or aborted the operation",
		},
		"run_advice_db_refused_fix": {
			PT: "Confira a mensagem técnica acima, execute Connection e corrija a configuração antes de repetir Migrate Run.",
			ES: "Revise el mensaje técnico de arriba, ejecute Connection y corrija la configuración antes de repetir Migrate Run.",
			EN: "Read the technical message above, run Connection and fix the configuration before repeating Migrate Run.",
		},
		"run_host_configured": {
			PT: "host configurado",
			ES: "host configurado",
			EN: "configured host",
		},

		// ── Catálogo e coerência entre migrations ──
		"run_alias_catalog_rebuild": {
			PT: "reconstruir catálogo inicial de aliases: %w",
			ES: "reconstruir el catálogo inicial de alias: %w",
			EN: "rebuilding the initial alias catalog: %w",
		},
		"run_alias_catalog_gen": {
			PT: "gerar catálogo de aliases no core: %w",
			ES: "generar el catálogo de alias en el core: %w",
			EN: "generating the alias catalog in the core: %w",
		},
		"run_view_catalog_gen": {
			PT: "gerar catálogo de views no core: %w",
			ES: "generar el catálogo de views en el core: %w",
			EN: "generating the view catalog in the core: %w",
		},
		"run_column_catalog_gen": {
			PT: "gerar catálogo de colunas no core: %w",
			ES: "generar el catálogo de columnas en el core: %w",
			EN: "generating the column catalog in the core: %w",
		},
		"run_dup_migration_id": {
			PT: "ID de migration duplicado %s, já usado por %s",
			ES: "ID de migración duplicado %s, ya usado por %s",
			EN: "duplicate migration ID %s, already used by %s",
		},
		"run_view_exists": {
			PT: "view %q já existe",
			ES: "la view %q ya existe",
			EN: "view %q already exists",
		},
		"run_alterview_needs_view": {
			PT: "AlterView exige uma view existente",
			ES: "AlterView exige una view existente",
			EN: "AlterView requires an existing view",
		},
		"run_dropview_needs_view": {
			PT: "DropView exige uma view existente",
			ES: "DropView exige una view existente",
			EN: "DropView requires an existing view",
		},
		"run_unknown_columns": {
			PT: "%s em %q usa a(s) coluna(s) %s, que nenhuma migration anterior criou",
			ES: "%s en %q usa la(s) columna(s) %s, que ninguna migración anterior creó",
			EN: "%s on %q uses column(s) %s, which no earlier migration created",
		},
		"run_constraint_name_used": {
			PT: "o nome de constraint/índice %q já foi usado em %s; nomes precisam ser únicos no schema",
			ES: "el nombre de constraint/índice %q ya fue usado en %s; los nombres deben ser únicos en el schema",
			EN: "the constraint/index name %q was already used in %s; names must be unique within the schema",
		},
		"run_alias_undeclared": {
			PT: "alias de tabela %q não foi declarado por nenhum CreateTable anterior",
			ES: "el alias de tabla %q no fue declarado por ningún CreateTable anterior",
			EN: "table alias %q was not declared by any earlier CreateTable",
		},
		"run_rename_physical_taken": {
			PT: "RenameTable usaria o nome físico %q já declarado em %s",
			ES: "RenameTable usaría el nombre físico %q ya declarado en %s",
			EN: "RenameTable would use the physical name %q already declared in %s",
		},
		"run_createtable_dup": {
			PT: "CreateTable duplicado para %q, já declarado em %s",
			ES: "CreateTable duplicado para %q, ya declarado en %s",
			EN: "duplicate CreateTable for %q, already declared in %s",
		},
		"run_alias_dup": {
			PT: "alias %q duplicado, já declarado em %s",
			ES: "alias %q duplicado, ya declarado en %s",
			EN: "duplicate alias %q, already declared in %s",
		},
		"run_fk_type_mismatch": {
			PT: "a FK %s.%s é %s mas %s.%s é %s; os dois lados precisam ter o mesmo tipo",
			ES: "la FK %s.%s es %s pero %s.%s es %s; los dos lados deben tener el mismo tipo",
			EN: "FK %s.%s is %s but %s.%s is %s; both sides must have the same type",
		},
		"run_fk_target_not_unique": {
			PT: "a FK de %s(%s) referencia %s(%s), mas essas colunas ainda não são PRIMARY KEY nem UNIQUE; declare AddUnique/AddPrimaryKey em %s antes desta migration",
			ES: "la FK de %s(%s) referencia %s(%s), pero esas columnas aún no son PRIMARY KEY ni UNIQUE; declare AddUnique/AddPrimaryKey en %s antes de esta migración",
			EN: "the FK on %s(%s) references %s(%s), but those columns are not PRIMARY KEY or UNIQUE yet; declare AddUnique/AddPrimaryKey on %s before this migration",
		},

		// ── Execução das operações ──
		"run_create_failed": {
			PT: "criar %s: %w",
			ES: "crear %s: %w",
			EN: "creating %s: %w",
		},
		"run_register_failed": {
			PT: "registrar %s: %w",
			ES: "registrar %s: %w",
			EN: "registering %s: %w",
		},
		"run_table_create_failed": {
			PT: "criar tabela %s: %w",
			ES: "crear la tabla %s: %w",
			EN: "creating table %s: %w",
		},
		"run_table_drop_failed": {
			PT: "remover tabela %s: %w",
			ES: "eliminar la tabla %s: %w",
			EN: "dropping table %s: %w",
		},
		"run_add_column_no_column": {
			PT: "add_column sem coluna",
			ES: "add_column sin columna",
			EN: "add_column without a column",
		},
		"run_rename_column_no_column": {
			PT: "rename_column sem coluna",
			ES: "rename_column sin columna",
			EN: "rename_column without a column",
		},
		"run_fk_no_definition": {
			PT: "add_foreign_key sem definição",
			ES: "add_foreign_key sin definición",
			EN: "add_foreign_key without a definition",
		},
		"run_op_add_column_invalid": {
			PT: "operação add_column inválida",
			ES: "operación add_column inválida",
			EN: "invalid add_column operation",
		},
		"run_op_alter_column_invalid": {
			PT: "operação alter_column inválida",
			ES: "operación alter_column inválida",
			EN: "invalid alter_column operation",
		},
		"run_op_drop_column_invalid": {
			PT: "operação drop_column inválida",
			ES: "operación drop_column inválida",
			EN: "invalid drop_column operation",
		},
		"run_op_add_fk_invalid": {
			PT: "operação add_foreign_key inválida",
			ES: "operación add_foreign_key inválida",
			EN: "invalid add_foreign_key operation",
		},
		"run_op_drop_fk_invalid": {
			PT: "operação drop_foreign_key inválida",
			ES: "operación drop_foreign_key inválida",
			EN: "invalid drop_foreign_key operation",
		},
		"run_op_rename_column_invalid": {
			PT: "operação rename_column inválida",
			ES: "operación rename_column inválida",
			EN: "invalid rename_column operation",
		},
		"run_column_add_failed": {
			PT: "adicionar %s.%s: %w",
			ES: "agregar %s.%s: %w",
			EN: "adding %s.%s: %w",
		},
		"run_column_alter_failed": {
			PT: "alterar %s.%s: %w",
			ES: "alterar %s.%s: %w",
			EN: "altering %s.%s: %w",
		},
		"run_column_drop_failed": {
			PT: "remover %s.%s: %w",
			ES: "eliminar %s.%s: %w",
			EN: "dropping %s.%s: %w",
		},
		"run_column_rename_failed": {
			PT: "renomear coluna %s.%s: %w",
			ES: "renombrar la columna %s.%s: %w",
			EN: "renaming column %s.%s: %w",
		},
		"run_table_rename_failed": {
			PT: "renomear tabela %s: %w",
			ES: "renombrar la tabla %s: %w",
			EN: "renaming table %s: %w",
		},
		"run_fk_create_failed": {
			PT: "criar relacionamento %s.%s: %w",
			ES: "crear la relación %s.%s: %w",
			EN: "creating relationship %s.%s: %w",
		},
		"run_fk_drop_failed": {
			PT: "remover relacionamento %s.%s: %w",
			ES: "eliminar la relación %s.%s: %w",
			EN: "dropping relationship %s.%s: %w",
		},
		"run_index_plan_failed": {
			PT: "planejar índice %s: %w",
			ES: "planificar el índice %s: %w",
			EN: "planning index %s: %w",
		},
		"run_index_create_failed": {
			PT: "criar índice %s: %w",
			ES: "crear el índice %s: %w",
			EN: "creating index %s: %w",
		},
		"run_index_drop_failed": {
			PT: "remover índice %s: %w",
			ES: "eliminar el índice %s: %w",
			EN: "dropping index %s: %w",
		},
		"run_view_create_exists": {
			PT: "criar view %s: a view já existe; use AlterView",
			ES: "crear la view %s: la view ya existe; use AlterView",
			EN: "creating view %s: the view already exists; use AlterView",
		},
		"run_view_create_sql_failed": {
			PT: "criar view %s usando %s.sql: %w; revise o SQL ou crie %s.sql para este dialeto",
			ES: "crear la view %s usando %s.sql: %w; revise el SQL o cree %s.sql para este dialecto",
			EN: "creating view %s using %s.sql: %w; review the SQL or create %s.sql for this dialect",
		},
		"run_view_alter_missing": {
			PT: "alterar view %s: a view não existe; use CreateView",
			ES: "alterar la view %s: la view no existe; use CreateView",
			EN: "altering view %s: the view does not exist; use CreateView",
		},
		"run_view_alter_sql_failed": {
			PT: "alterar view %s usando %s.sql: %w; revise o SQL ou crie %s.sql para este dialeto",
			ES: "alterar la view %s usando %s.sql: %w; revise el SQL o cree %s.sql para este dialecto",
			EN: "altering view %s using %s.sql: %w; review the SQL or create %s.sql for this dialect",
		},
		"run_view_drop_missing": {
			PT: "remover view %s: a view não existe",
			ES: "eliminar la view %s: la view no existe",
			EN: "dropping view %s: the view does not exist",
		},
		"run_view_drop_failed": {
			PT: "remover view %s: %w",
			ES: "eliminar la view %s: %w",
			EN: "dropping view %s: %w",
		},
		"run_sequence_mysql": {
			PT: "sequences não são suportadas pelo MySQL",
			ES: "las sequences no son soportadas por MySQL",
			EN: "sequences are not supported by MySQL",
		},
		"run_sequence_create_failed": {
			PT: "criar sequence %s: %w",
			ES: "crear la sequence %s: %w",
			EN: "creating sequence %s: %w",
		},
		"run_sequence_drop_failed": {
			PT: "remover sequence %s: %w",
			ES: "eliminar la sequence %s: %w",
			EN: "dropping sequence %s: %w",
		},
		"run_pk_prepare_failed": {
			PT: "preparar %s.%s para a chave primária: %w",
			ES: "preparar %s.%s para la clave primaria: %w",
			EN: "preparing %s.%s for the primary key: %w",
		},
		"run_constraint_create_failed": {
			PT: "criar constraint %s: %w",
			ES: "crear la constraint %s: %w",
			EN: "creating constraint %s: %w",
		},
		"run_check_create_failed": {
			PT: "criar check %s: %w",
			ES: "crear el check %s: %w",
			EN: "creating check %s: %w",
		},
		"run_constraint_drop_failed": {
			PT: "remover constraint %s: %w",
			ES: "eliminar la constraint %s: %w",
			EN: "dropping constraint %s: %w",
		},
		"run_dialect_sql_stmt_failed": {
			PT: "executar SQL específico de %s (statement %d de %d): %w",
			ES: "ejecutar SQL específico de %s (statement %d de %d): %w",
			EN: "running %s-specific SQL (statement %d of %d): %w",
		},
		"run_dialect_sql_failed": {
			PT: "executar SQL específico de %s: %w",
			ES: "ejecutar SQL específico de %s: %w",
			EN: "running %s-specific SQL: %w",
		},
		"run_op_unknown": {
			PT: "operação desconhecida: %s",
			ES: "operación desconocida: %s",
			EN: "unknown operation: %s",
		},
		"run_op_kind_unknown": {
			PT: "tipo de operação de migração desconhecido: %s",
			ES: "tipo de operación de migración desconocido: %s",
			EN: "unknown migration operation type: %s",
		},
		"run_column_not_found": {
			PT: "coluna não encontrada",
			ES: "columna no encontrada",
			EN: "column not found",
		},
		"run_column_missing": {
			PT: "coluna %s.%s não existe",
			ES: "la columna %s.%s no existe",
			EN: "column %s.%s does not exist",
		},
		"run_table_no_columns": {
			PT: "tabela %s não possui colunas",
			ES: "la tabla %s no tiene columnas",
			EN: "table %s has no columns",
		},
		"run_view_no_sql": {
			PT: "view %s não possui SQL para %s nem common.sql",
			ES: "la view %s no tiene SQL para %s ni common.sql",
			EN: "view %s has no SQL for %s nor a common.sql",
		},

		// ── Seed ──
		"run_seed_tx_failed": {
			PT: "iniciar transação do seed de %s: %w",
			ES: "iniciar la transacción del seed de %s: %w",
			EN: "starting the seed transaction for %s: %w",
		},
		"run_seed_insert_failed": {
			PT: "inserir em %s (linha %d): %w",
			ES: "insertar en %s (fila %d): %w",
			EN: "inserting into %s (row %d): %w",
		},
		"run_seed_query_failed": {
			PT: "consultar %s (linha %d): %w",
			ES: "consultar %s (fila %d): %w",
			EN: "querying %s (row %d): %w",
		},
		"run_seed_update_failed": {
			PT: "atualizar %s (linha %d): %w",
			ES: "actualizar %s (fila %d): %w",
			EN: "updating %s (row %d): %w",
		},
		"run_seed_commit_failed": {
			PT: "confirmar seed de %s: %w",
			ES: "confirmar el seed de %s: %w",
			EN: "committing the seed for %s: %w",
		},
		"run_seed_sequence_failed": {
			PT: "ressincronizar a sequência de %s.%s: %w",
			ES: "resincronizar la secuencia de %s.%s: %w",
			EN: "resyncing the sequence of %s.%s: %w",
		},
		"run_seed_summary": {
			PT: "  %s %s: %d inserida(s), %d editada(s), %d inalterada(s)\n",
			ES: "  %s %s: %d insertada(s), %d editada(s), %d sin cambios\n",
			EN: "  %s %s: %d inserted, %d updated, %d unchanged\n",
		},
		"run_seed_difference": {
			PT: "%s: banco tem %q, seed declara %q",
			ES: "%s: la base tiene %q, el seed declara %q",
			EN: "%s: the database has %q, the seed declares %q",
		},

		// ── Scaffold de migration ──
		"run_mig_folder_failed": {
			PT: "criar pasta de migrations: %w",
			ES: "crear la carpeta de migraciones: %w",
			EN: "creating the migrations folder: %w",
		},
		"run_view_folder_failed": {
			PT: "criar pasta de views: %w",
			ES: "crear la carpeta de views: %w",
			EN: "creating the views folder: %w",
		},
		"run_view_sql_write_failed": {
			PT: "escrever arquivo SQL da view: %w",
			ES: "escribir el archivo SQL de la view: %w",
			EN: "writing the view's SQL file: %w",
		},
		"run_sql_placeholder": {
			PT: "-- escreva o SQL aqui",
			ES: "-- escriba el SQL aquí",
			EN: "-- write the SQL here",
		},
		"run_mig_write_failed": {
			PT: "escrever arquivo de migration: %w",
			ES: "escribir el archivo de migración: %w",
			EN: "writing the migration file: %w",
		},
		"run_mig_register_failed": {
			PT: "registrar migration gerada: %w",
			ES: "registrar la migración generada: %w",
			EN: "registering the generated migration: %w",
		},
		// Avisos, não falhas: a migration já está criada quando isto acontece. O caso
		// normal é o scaffold recém-criado ainda não estar preenchido, então o corpus
		// não descreve um schema válido — e aí regerar de propósito não dá certo.
		"run_regen_factory_skipped": {
			PT: "a migration foi criada, mas as factories não pôderam ser atualizadas agora (%w); rode `gokit factory create` depois de preencher a migration",
			ES: "la migración fue creada, pero las factories no pudieron actualizarse ahora (%w); ejecute `gokit factory create` después de completar la migración",
			EN: "the migration was created, but the factories could not be updated now (%w); run `gokit factory create` after filling the migration in",
		},
		"run_module_not_found": {
			PT: "nome do módulo não encontrado em go.mod",
			ES: "nombre del módulo no encontrado en go.mod",
			EN: "module name not found in go.mod",
		},
	})
}

// Cabeçalho do grupo de índices que o run tolerou. Info, não erro.
//
// Ele NÃO declara a causa: o mesmo grupo reúne motivos com consequências opostas — na
// redundância a lista está indexada com outro nome, no limite de colunas e no tipo grande
// não existe índice nenhum. A versão anterior afirmava "a lista de colunas já estava
// indexada (só o Oracle recusa)" e logo abaixo listava itens onde nada foi indexado,
// rodando em SQL Server. O motivo de cada item vem em cada linha, que é onde ele é
// verdade; o que o cabeçalho garante é só o que vale para todos: nenhum foi criado com o
// nome declarado, então rollback por nome não acha.
func init() {
	Register(Entradas{
		"run_index_redundant": {
			PT: "%d índice(s) não criados, cada um pelo motivo indicado. O nome declarado NÃO existe no banco — um rollback por nome não vai encontrá-lo:",
			ES: "%d índice(s) no creados, cada uno por el motivo indicado. El nombre declarado NO existe en la base — un rollback por nombre no lo encontrará:",
			EN: "%d index(es) not created, each for the reason shown. The declared name does NOT exist in the database — a rollback by name will not find it:",
		},
	})
}

// Motivos pelos quais um índice não foi criado e o run seguiu. Ficam separados
// porque a consequência é diferente: no primeiro a lista está indexada com outro
// nome; no segundo não há índice nenhum.
func init() {
	Register(Entradas{
		"run_index_skip_redundant": {
			PT: "a lista de colunas já estava indexada; o nome declarado não existe no banco",
			ES: "la lista de columnas ya estaba indexada; el nombre declarado no existe en la base",
			EN: "the column list was already indexed; the declared name does not exist in the database",
		},
		// O limite existe nos QUATRO, com tetos diferentes (MySQL 16, os outros três 32).
		// A mensagem não nomeia dialeto de propósito: nomear era mentira quando o mesmo
		// aviso saía rodando em SQL Server dizendo "do Oracle".
		"run_index_skip_too_many": {
			PT: "colunas acima do teto de índice deste banco; NENHUM índice foi criado, é desempenho perdido",
			ES: "columnas por encima del tope de índice de esta base; NINGÚN índice fue creado, es rendimiento perdido",
			EN: "more columns than this database's index ceiling; NO index was created — this is lost performance",
		},
	})
}

func init() {
	Register(Entradas{
		// Também não é exclusivo do Oracle: no SQL Server text/ntext/image/xml e
		// varchar(max) são igualmente inválidos como coluna de chave.
		"run_index_skip_lob": {
			PT: "coluna de tipo grande (LOB/CLOB/BLOB/text/xml) não é indexável neste banco; NENHUM índice foi criado, é desempenho perdido",
			ES: "columna de tipo grande (LOB/CLOB/BLOB/text/xml) no es indexable en esta base; NINGÚN índice fue creado, es rendimiento perdido",
			EN: "a large-type column (LOB/CLOB/BLOB/text/xml) cannot be indexed on this database; NO index was created — this is lost performance",
		},
	})
}

// Promoção de VARCHAR/CHAR para TEXT no MySQL. WARNING, não info: a semântica daquela
// tabela passa a ser diferente dos outros três dialetos, e quem lê o schema pelo banco
// precisa saber que a diferença veio do gokit e não do corpus.
func init() {
	Register(Entradas{
		"run_mysql_text_promotion": {
			PT: "%d tabela(s) não caberiam no teto de 65.535 bytes por linha do MySQL: as colunas de texto mais largas foram criadas como TEXT. Nessas tabelas o MySQL DIFERE dos outros três — TEXT não aceita DEFAULT literal nem índice sem prefixo:",
			ES: "%d tabla(s) no cabrían en el tope de 65.535 bytes por fila de MySQL: las columnas de texto más anchas fueron creadas como TEXT. En esas tablas MySQL DIFIERE de los otros tres — TEXT no acepta DEFAULT literal ni índice sin prefijo:",
			EN: "%d table(s) would not fit MySQL's 65,535-byte row ceiling: the widest text columns were created as TEXT. In those tables MySQL DIFFERS from the other three — TEXT accepts neither a literal DEFAULT nor an index without a prefix:",
		},
	})
}

// FK que aponta a coluna para ela mesma. Pulada nos quatro: não restringe nada.
func init() {
	Register(Entradas{
		"run_fk_tautologica": {
			PT: "%d chave(s) estrangeira(s) não criadas: a coluna referencia ELA MESMA na própria tabela, o que não restringe nada. Existem no banco de origem; o SQL Server aceita e o MySQL recusa. Puladas nos quatro para o schema sair igual:",
			ES: "%d clave(s) foránea(s) no creadas: la columna referencia A SÍ MISMA en su propia tabla, lo que no restringe nada. Existen en la base de origen; SQL Server las acepta y MySQL las rechaza. Omitidas en los cuatro para que el esquema salga igual:",
			EN: "%d foreign key(s) not created: the column references ITSELF in its own table, which constrains nothing. They exist in the source database; SQL Server accepts them and MySQL refuses. Skipped on all four so the schema comes out the same:",
		},
	})
}
