package i18n

// Mensagens da leitura do esquema do banco (migrate scan): o caminho inverso, em
// que o gokit olha o banco existente em vez de aplicar o que foi declarado.
func init() {
	Register(Entradas{
		"scan_header": {
			PT: "Lendo o esquema de %s (%s): %d tabela(s) encontrada(s)",
			ES: "Leyendo el esquema de %s (%s): %d tabla(s) encontrada(s)",
			EN: "Reading the schema of %s (%s): %d table(s) found",
		},
		"scan_in_sync": {
			PT: "banco e migrations descrevem as mesmas tabelas",
			ES: "la base y las migrations describen las mismas tablas",
			EN: "database and migrations describe the same tables",
		},
		"scan_only_in_db": {
			PT: "%d tabela(s) existem no banco e NÃO têm migration:",
			ES: "%d tabla(s) existen en la base y NO tienen migration:",
			EN: "%d table(s) exist in the database and have NO migration:",
		},
		"scan_only_in_corpus": {
			PT: "%d tabela(s) declaradas nas migrations e ausentes do banco:",
			ES: "%d tabla(s) declaradas en las migrations y ausentes de la base:",
			EN: "%d table(s) declared in the migrations and missing from the database:",
		},
		"scan_table_summary": {
			PT: "%d coluna(s), %d na chave, %d FK",
			ES: "%d columna(s), %d en la clave, %d FK",
			EN: "%d column(s), %d in the key, %d FK",
		},
		"scan_hint_import": {
			PT: "Use `gokit migrate scan --detail` para ver as colunas, ou `gokit migrate import` para escrever as migrations.",
			ES: "Use `gokit migrate scan --detail` para ver las columnas, o `gokit migrate import` para escribir las migrations.",
			EN: "Run `gokit migrate scan --detail` to see the columns, or `gokit migrate import` to write the migrations.",
		},
		"scan_hint_pending": {
			PT: "Isso é migration pendente ou aplicada em outro ambiente. Rode `gokit migrate run`.",
			ES: "Eso es migration pendiente o aplicada en otro entorno. Ejecute `gokit migrate run`.",
			EN: "That is a pending migration, or one applied in another environment. Run `gokit migrate run`.",
		},
		"scan_no_config": {
			PT: "não há configuração carregada para ler o banco",
			ES: "no hay configuración cargada para leer la base",
			EN: "no configuration loaded to read the database",
		},
	})
}

// Mensagens da escrita da migration a partir do banco (migrate import).
func init() {
	Register(Entradas{
		"imp_header": {
			PT: "%d migration(s) a escrever, na ordem de dependência:",
			ES: "%d migration(s) a escribir, en orden de dependencia:",
			EN: "%d migration(s) to write, in dependency order:",
		},
		"imp_table_summary": {
			PT: "tabela %s, %d coluna(s)",
			ES: "tabla %s, %d columna(s)",
			EN: "table %s, %d column(s)",
		},
		"imp_preview_rest": {
			PT: "... e mais %d arquivo(s) no mesmo formato.",
			ES: "... y %d archivo(s) más en el mismo formato.",
			EN: "... and %d more file(s) in the same format.",
		},
		"imp_needs_confirm": {
			PT: "Nada foi escrito. Rode `gokit migrate import --confirm` para gravar.",
			ES: "Nada fue escrito. Ejecute `gokit migrate import --confirm` para grabar.",
			EN: "Nothing was written. Run `gokit migrate import --confirm` to write.",
		},
		"imp_written": {
			PT: "%d migration(s) escrita(s). Confira os arquivos e rode `gokit migrate validate`.",
			ES: "%d migration(s) escrita(s). Revise los archivos y ejecute `gokit migrate validate`.",
			EN: "%d migration(s) written. Review the files and run `gokit migrate validate`.",
		},
		"imp_file_exists": {
			PT: "o arquivo %s já existe",
			ES: "el archivo %s ya existe",
			EN: "file %s already exists",
		},
		"imp_file_exists_fix": {
			PT: "Nenhum arquivo é sobrescrito pelo import. Apague o arquivo ou renomeie-o antes de importar de novo.",
			ES: "Ningún archivo es sobrescrito por el import. Borre el archivo o renómbrelo antes de importar de nuevo.",
			EN: "The import never overwrites a file. Delete or rename it before importing again.",
		},
		"imp_gen_doc": {
			PT: "// Migration gerada a partir da tabela %s que já existia no banco.\n//\n// Revise antes de aplicar. O import cobre colunas, tipo, nulidade, chave primária,\n// identidade, chave estrangeira, DEFAULT e índice. NÃO cobre CHECK, coluna computed\n// nem view — o que estiver só no banco não aparece aqui.\n",
			ES: "// Migration generada a partir de la tabla %s que ya existía en la base.\n//\n// Revise antes de aplicar. El import cubre columnas, tipo, nulidad, clave primaria,\n// identidad, clave foránea, DEFAULT e índice. NO cubre CHECK, columna computed\n// ni view — lo que esté solo en la base no aparece aquí.\n",
			EN: "// Migration generated from table %s, which already existed in the database.\n//\n// Review before applying. The import covers columns, type, nullability, primary key,\n// identity, foreign key, DEFAULT and indexes. It does NOT cover CHECK, computed\n// columns or views — anything declared only in the database is absent here.\n",
		},
	})
}

func init() {
	Register(Entradas{
		"imp_format_failed": {
			PT: "a migration gerada para %s não é Go válido: %w",
			ES: "la migration generada para %s no es Go válido: %w",
			EN: "the migration generated for %s is not valid Go: %w",
		},
	})
}

func init() {
	Register(Entradas{
		"imp_identity_warning": {
			PT: "%d tabela(s) têm coluna de identidade FORA da chave primária. A declaração sai fiel ao\nbanco, mas saiba o que acontece ao aplicar: o MySQL não aceita coluna AUTO_INCREMENT que\nnão seja chave, então o gokit cria uma UNIQUE KEY para ela — só no MySQL, e a única no\nesquema que não veio da origem:",
			ES: "%d tabla(s) tienen columna de identidad FUERA de la clave primaria. La declaración sale fiel\na la base, pero sepa qué pasa al aplicar: MySQL no acepta columna AUTO_INCREMENT que no sea\nclave, así que GoKit crea una UNIQUE KEY para ella — solo en MySQL, y la única del esquema\nque no vino del origen:",
			EN: "%d table(s) have an identity column OUTSIDE the primary key. The declaration stays faithful\nto the database, but know what happens on apply: MySQL rejects an AUTO_INCREMENT column that\nis not a key, so GoKit creates a UNIQUE KEY for it — on MySQL only, and the only thing in the\nschema that did not come from the source:",
		},
	})
}

func init() {
	Register(Entradas{
		"imp_catalog_failed": {
			PT: "as migrations foram escritas, mas o catálogo do core não pôde ser atualizado: %w",
			ES: "las migrations fueron escritas, pero el catálogo del core no pudo actualizarse: %w",
			EN: "the migrations were written, but the core catalog could not be refreshed: %w",
		},
	})
}

// Mensagens do monitor: o registro do que a leitura do banco não carregou.
func init() {
	Register(Entradas{
		"mon_resumo": {
			PT: "%d ocorrência(s) que o import não carrega com fidelidade:",
			ES: "%d ocurrencia(s) que el import no carga con fidelidad:",
			EN: "%d occurrence(s) the import does not carry faithfully:",
		},
		"mon_gravado": {
			PT: "%d ocorrência(s) registradas em %s — é a lista de trabalho, não erro:",
			ES: "%d ocurrencia(s) registradas en %s — es la lista de trabajo, no error:",
			EN: "%d occurrence(s) recorded in %s — this is the work list, not an error:",
		},
		"mon_falhou": {
			PT: "as migrations foram escritas, mas o registro do monitor não pôde ser gravado: %v",
			ES: "las migrations fueron escritas, pero el registro del monitor no pudo grabarse: %v",
			EN: "the migrations were written, but the monitor record could not be saved: %v",
		},
	})
}

// Mensagens do import de views.
func init() {
	Register(Entradas{
		"imp_view_summary": {
			PT: "view %s, definição de %s",
			ES: "view %s, definición de %s",
			EN: "view %s, definition from %s",
		},
		"imp_view_doc": {
			PT: "// View %s importada do banco.\n//\n// A definição está em views/%[1]s/<timestamp>/%[2]s.sql — o arquivo do dialeto de\n// ORIGEM, não common.sql. Aquele SQL funciona em %[2]s e NÃO foi verificado nos\n// outros três: nenhuma conversão automática foi feita, porque TOP, ROWNUM e LIMIT\n// não têm tradução confiável e converter errado gravaria mentira no histórico.\n//\n// Para rodar nos outros dialetos, escreva o .sql de cada um, ou renomeie para\n// common.sql se o SQL for portável de fato.\n",
			ES: "// View %s importada de la base.\n//\n// La definición está en views/%[1]s/<timestamp>/%[2]s.sql — el archivo del dialecto\n// de ORIGEN, no common.sql. Ese SQL funciona en %[2]s y NO fue verificado en los\n// otros tres: no se hizo ninguna conversión automática, porque TOP, ROWNUM y LIMIT\n// no tienen traducción confiable y convertir mal grabaría mentira en el historial.\n//\n// Para ejecutar en los otros dialectos, escriba el .sql de cada uno, o renombre a\n// common.sql si el SQL es portable de hecho.\n",
			EN: "// View %s imported from the database.\n//\n// The definition lives in views/%[1]s/<timestamp>/%[2]s.sql — the SOURCE dialect's\n// file, not common.sql. That SQL works on %[2]s and was NOT verified on the other\n// three: no automatic conversion was made, because TOP, ROWNUM and LIMIT have no\n// reliable translation and converting wrong would write a lie into history.\n//\n// To run on the other dialects, write each one's .sql, or rename to common.sql if\n// the SQL really is portable.\n",
		},
	})
}

func init() {
	Register(Entradas{
		"scan_views_only_in_db": {
			PT: "%d view(s) existem no banco e NÃO têm migration:",
			ES: "%d view(s) existen en la base y NO tienen migration:",
			EN: "%d view(s) exist in the database and have NO migration:",
		},
		"scan_view_summary": {
			PT: "definição com %d linha(s)",
			ES: "definición con %d línea(s)",
			EN: "definition with %d line(s)",
		},
		"scan_hint_view": {
			PT: "O import grava a definição no arquivo do dialeto de ORIGEM, sem converter: SQL de view raramente é portável.",
			ES: "El import graba la definición en el archivo del dialecto de ORIGEN, sin convertir: el SQL de view raramente es portable.",
			EN: "The import writes the definition into the SOURCE dialect's file, without converting: view SQL is rarely portable.",
		},
	})
}

// Mensagens do baseline: marcar migration como aplicada sem executar.
func init() {
	Register(Entradas{
		"bas_sem_pendentes": {
			PT: "não há migration pendente para marcar",
			ES: "no hay migration pendiente para marcar",
			EN: "there is no pending migration to mark",
		},
		"bas_cabecalho": {
			PT: "%d migration(s) pendente(s) em %s (%s). Cada uma é conferida no banco antes de ser marcada:",
			ES: "%d migration(s) pendiente(s) en %s (%s). Cada una es verificada en la base antes de marcarse:",
			EN: "%d pending migration(s) on %s (%s). Each one is checked against the database before being marked:",
		},
		"bas_objetos_existem": {
			PT: "%d objeto(s) conferido(s) — pode ser marcada",
			ES: "%d objeto(s) verificado(s) — puede marcarse",
			EN: "%d object(s) verified — can be marked",
		},
		"bas_objetos_faltando": {
			PT: "continua pendente, falta no banco: %s",
			ES: "sigue pendiente, falta en la base: %s",
			EN: "still pending, missing from the database: %s",
		},
		"bas_operacao_nao_conferivel": {
			PT: "operação %s, que este conferidor não sabe avaliar",
			ES: "operación %s, que este verificador no sabe evaluar",
			EN: "operation %s, which this checker cannot evaluate",
		},
		"bas_nada_a_marcar": {
			PT: "Nenhuma migration passou na verificação: o banco não tem os objetos que elas declaram.\nIsso é trabalho para `gokit migrate run`, não para o baseline.",
			ES: "Ninguna migration pasó la verificación: la base no tiene los objetos que declaran.\nEso es trabajo para `gokit migrate run`, no para el baseline.",
			EN: "No migration passed verification: the database lacks the objects they declare.\nThat is work for `gokit migrate run`, not for the baseline.",
		},
		"bas_precisa_confirmar": {
			PT: "%d migration(s) podem ser marcadas como aplicadas e %d continuam pendentes.\nNada foi gravado. Rode `gokit migrate baseline --confirm` para marcar.",
			ES: "%d migration(s) pueden marcarse como aplicadas y %d siguen pendientes.\nNada fue grabado. Ejecute `gokit migrate baseline --confirm` para marcar.",
			EN: "%d migration(s) can be marked as applied and %d remain pending.\nNothing was written. Run `gokit migrate baseline --confirm` to mark them.",
		},
		"bas_marcadas": {
			PT: "%d migration(s) marcada(s) como aplicada(s) sem executar; %d continuam pendentes para o `migrate run`.",
			ES: "%d migration(s) marcada(s) como aplicada(s) sin ejecutar; %d siguen pendientes para el `migrate run`.",
			EN: "%d migration(s) marked as applied without executing; %d remain pending for `migrate run`.",
		},
	})
}

func init() {
	Register(Entradas{
		"cfg_create_failed": {
			PT: "não foi possível criar a configuração do gokit: %w",
			ES: "no fue posible crear la configuración de gokit: %w",
			EN: "could not create the gokit configuration: %w",
		},
	})
}
