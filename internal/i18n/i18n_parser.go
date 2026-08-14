package i18n

// Mensagens do parser de migrations e seeders: a gramática do DSL em AST.
// Cada frase aponta o que a migration deveria ter escrito — é o erro que o
// desenvolvedor lê antes de o gokit tocar em qualquer banco.
func init() {
	Register(Entradas{
		// ── Estrutura do arquivo ──
		"mgp_seeder_wrong_place": {
			PT: "func Seeder() não pertence a uma migration; mova as linhas para %s/<tabela>/<timestamp>_seeder.go (veja: gokit seed create <tabela>)",
			ES: "func Seeder() no pertenece a una migración; mueva las filas a %s/<tabla>/<timestamp>_seeder.go (vea: gokit seed create <tabla>)",
			EN: "func Seeder() does not belong in a migration; move the rows to %s/<table>/<timestamp>_seeder.go (see: gokit seed create <table>)",
		},
		"mgp_no_operation": {
			PT: "nenhuma operação migrate.* encontrada",
			ES: "ninguna operación migrate.* encontrada",
			EN: "no migrate.* operation found",
		},
		"mgp_createtable_needs_alias": {
			PT: "CreateTable exige .Alias(\"apelido\")",
			ES: "CreateTable exige .Alias(\"apodo\")",
			EN: "CreateTable requires .Alias(\"nickname\")",
		},
		"mgp_define_needs_action": {
			PT: "migrate.Define exige ao menos uma ação",
			ES: "migrate.Define exige al menos una acción",
			EN: "migrate.Define requires at least one action",
		},
		"mgp_no_seeder_func": {
			PT: "nenhuma func Seeder() encontrada",
			ES: "ninguna func Seeder() encontrada",
			EN: "no func Seeder() found",
		},

		// ── Corpo do Seeder ──
		"mgp_seeder_body": {
			PT: "Seeder() deve conter apenas `return migrate.Rows{...}`",
			ES: "Seeder() debe contener solo `return migrate.Rows{...}`",
			EN: "Seeder() must contain only `return migrate.Rows{...}`",
		},
		"mgp_seeder_one_list": {
			PT: "Seeder() deve retornar exatamente uma lista migrate.Rows",
			ES: "Seeder() debe retornar exactamente una lista migrate.Rows",
			EN: "Seeder() must return exactly one migrate.Rows list",
		},
		"mgp_seeder_literal": {
			PT: "Seeder() deve retornar um literal migrate.Rows{...}",
			ES: "Seeder() debe retornar un literal migrate.Rows{...}",
			EN: "Seeder() must return a migrate.Rows{...} literal",
		},
		"mgp_row_literal": {
			PT: "a linha %d deve ser um literal {\"coluna\": valor, ...}",
			ES: "la fila %d debe ser un literal {\"columna\": valor, ...}",
			EN: "row %d must be a {\"column\": value, ...} literal",
		},
		"mgp_row_format": {
			PT: "a linha %d deve usar o formato \"coluna\": valor",
			ES: "la fila %d debe usar el formato \"columna\": valor",
			EN: "row %d must use the \"column\": value format",
		},
		"mgp_row_bad_column": {
			PT: "a linha %d tem um nome de coluna inválido; use \"coluna\": valor",
			ES: "la fila %d tiene un nombre de columna inválido; use \"columna\": valor",
			EN: "row %d has an invalid column name; use \"column\": value",
		},
		"mgp_row_dup_column": {
			PT: "a linha %d repete a coluna %q",
			ES: "la fila %d repite la columna %q",
			EN: "row %d repeats column %q",
		},
		"mgp_row_column_detail": {
			PT: "linha %d, coluna %q: %v",
			ES: "fila %d, columna %q: %v",
			EN: "row %d, column %q: %v",
		},
		"mgp_row_column_wrap": {
			PT: "linha %d, coluna %q: %w",
			ES: "fila %d, columna %q: %w",
			EN: "row %d, column %q: %w",
		},
		"mgp_only_time_call": {
			PT: "só migrate.Time(\"...\") é aceito como chamada em valor de seed",
			ES: "solo migrate.Time(\"...\") es aceptado como llamada en un valor de seed",
			EN: "only migrate.Time(\"...\") is accepted as a call in a seed value",
		},
		"mgp_time_one_text": {
			PT: "migrate.Time exige exatamente um texto",
			ES: "migrate.Time exige exactamente un texto",
			EN: "migrate.Time requires exactly one string",
		},
		"mgp_time_quoted": {
			PT: "migrate.Time exige um texto entre aspas",
			ES: "migrate.Time exige un texto entre comillas",
			EN: "migrate.Time requires a quoted string",
		},
		"mgp_time_format": {
			PT: "migrate.Time(%q) fora do formato %s",
			ES: "migrate.Time(%q) fuera del formato %s",
			EN: "migrate.Time(%q) does not match the format %s",
		},
		"mgp_bad_value": {
			PT: "valor inválido; use texto, número, true, false ou nil",
			ES: "valor inválido; use texto, número, true, false o nil",
			EN: "invalid value; use a string, a number, true, false or nil",
		},
		"mgp_bad_date": {
			PT: "%q não é uma data válida; use o formato %s",
			ES: "%q no es una fecha válida; use el formato %s",
			EN: "%q is not a valid date; use the format %s",
		},
		"mgp_bad_bool": {
			PT: "%q não é um booleano válido (use true/false ou 0/1)",
			ES: "%q no es un booleano válido (use true/false o 0/1)",
			EN: "%q is not a valid boolean (use true/false or 0/1)",
		},

		// ── Gramática das ações ──
		"mgp_expect_method": {
			PT: "esperado migrate.Metodo(...)",
			ES: "se esperaba migrate.Metodo(...)",
			EN: "expected migrate.Method(...)",
		},
		"mgp_alias_one_arg": {
			PT: "Alias exige exatamente um apelido",
			ES: "Alias exige exactamente un apodo",
			EN: "Alias requires exactly one nickname",
		},
		"mgp_alias_only_create": {
			PT: "Alias só pode ser usado em CreateTable",
			ES: "Alias solo puede usarse en CreateTable",
			EN: "Alias can only be used on CreateTable",
		},
		"mgp_createtable_args": {
			PT: "CreateTable exige nome e colunas",
			ES: "CreateTable exige nombre y columnas",
			EN: "CreateTable requires a name and columns",
		},
		"mgp_needs_alias_ref": {
			PT: "%s exige core.Table.*",
			ES: "%s exige core.Table.*",
			EN: "%s requires core.Table.*",
		},
		"mgp_renametable_args": {
			PT: "RenameTable exige core.Table.* e novo nome",
			ES: "RenameTable exige core.Table.* y nuevo nombre",
			EN: "RenameTable requires core.Table.* and a new name",
		},
		"mgp_renamecolumn_args": {
			PT: "RenameColumn exige core.Table.*, nome atual e novo nome",
			ES: "RenameColumn exige core.Table.*, nombre actual y nuevo nombre",
			EN: "RenameColumn requires core.Table.*, the current name and the new name",
		},
		"mgp_needs_alias_name_columns": {
			PT: "%s exige core.Table.*, nome e colunas",
			ES: "%s exige core.Table.*, nombre y columnas",
			EN: "%s requires core.Table.*, a name and columns",
		},
		"mgp_composite_fk_args": {
			PT: "AddCompositeForeignKey exige core.Table.*, nome, tabela referenciada e mapeamentos",
			ES: "AddCompositeForeignKey exige core.Table.*, nombre, tabla referenciada y mapeos",
			EN: "AddCompositeForeignKey requires core.Table.*, a name, the referenced table and the mappings",
		},
		"mgp_mapping_format": {
			PT: "mapeamento %q deve usar coluna:referência",
			ES: "el mapeo %q debe usar columna:referencia",
			EN: "mapping %q must use column:reference",
		},
		"mgp_addcheck_args": {
			PT: "AddCheck exige core.Table.*, nome e expressão",
			ES: "AddCheck exige core.Table.*, nombre y expresión",
			EN: "AddCheck requires core.Table.*, a name and an expression",
		},
		"mgp_dropconstraint_args": {
			PT: "DropConstraint exige core.Table.* e nome",
			ES: "DropConstraint exige core.Table.* y nombre",
			EN: "DropConstraint requires core.Table.* and a name",
		},
		"mgp_dropindex_args": {
			PT: "DropIndex exige core.Table.* e nome",
			ES: "DropIndex exige core.Table.* y nombre",
			EN: "DropIndex requires core.Table.* and a name",
		},
		"mgp_method_unsupported": {
			PT: "método migrate.%s não suportado",
			ES: "método migrate.%s no soportado",
			EN: "unsupported migrate.%s method",
		},
		"mgp_use_alias_ref": {
			PT: "use uma referência core.Table.*",
			ES: "use una referencia core.Table.*",
			EN: "use a core.Table.* reference",
		},
		"mgp_alias_not_in_catalog": {
			PT: "referência core.Table.%s não existe no catálogo",
			ES: "la referencia core.Table.%s no existe en el catálogo",
			EN: "reference core.Table.%s does not exist in the catalog",
		},
		"mgp_use_view_ref": {
			PT: "use exclusivamente uma referência view.*",
			ES: "use exclusivamente una referencia view.*",
			EN: "use a view.* reference exclusively",
		},
		"mgp_view_not_in_catalog": {
			PT: "referência view.%s não existe no catálogo",
			ES: "la referencia view.%s no existe en el catálogo",
			EN: "reference view.%s does not exist in the catalog",
		},
		"mgp_view_needs_timestamp": {
			PT: "migration de view precisa iniciar com um ID de timestamp",
			ES: "una migración de view necesita iniciar con un ID de timestamp",
			EN: "a view migration must start with a timestamp ID",
		},
		"mgp_view_sql_missing": {
			PT: "SQL da view %s não encontrado para a migration %s",
			ES: "SQL de la view %s no encontrado para la migración %s",
			EN: "SQL for view %s not found for migration %s",
		},
		"mgp_view_file_invalid": {
			PT: "arquivo de view %s inválido; use common.sql, oracle.sql, postgres.sql, mysql.sql ou sqlserver.sql",
			ES: "archivo de view %s inválido; use common.sql, oracle.sql, postgres.sql, mysql.sql o sqlserver.sql",
			EN: "invalid view file %s; use common.sql, oracle.sql, postgres.sql, mysql.sql or sqlserver.sql",
		},
		"mgp_file_empty": {
			PT: "%s está vazio",
			ES: "%s está vacío",
			EN: "%s is empty",
		},
		"mgp_no_sql_file": {
			PT: "%s não contém nenhum .sql; crie common.sql ou um arquivo por dialeto (oracle.sql, postgres.sql, mysql.sql, sqlserver.sql)",
			ES: "%s no contiene ningún .sql; cree common.sql o un archivo por dialecto (oracle.sql, postgres.sql, mysql.sql, sqlserver.sql)",
			EN: "%s contains no .sql; create common.sql or one file per dialect (oracle.sql, postgres.sql, mysql.sql, sqlserver.sql)",
		},
		"mgp_legacy_action": {
			PT: "ação antiga inválida",
			ES: "acción antigua inválida",
			EN: "invalid legacy action",
		},

		// ── Gramática das colunas ──
		"mgp_expect_col": {
			PT: "esperado col(...) ou método de coluna",
			ES: "se esperaba col(...) o un método de columna",
			EN: "expected col(...) or a column method",
		},
		"mgp_col_needs_name": {
			PT: "col exige um nome",
			ES: "col exige un nombre",
			EN: "col requires a name",
		},
		"mgp_col_bad_name": {
			PT: "nome de coluna inválido",
			ES: "nombre de columna inválido",
			EN: "invalid column name",
		},
		"mgp_varchar_size": {
			PT: "Varchar exige tamanho",
			ES: "Varchar exige tamaño",
			EN: "Varchar requires a size",
		},
		"mgp_varchar_size_numeric": {
			PT: "Varchar exige tamanho numérico",
			ES: "Varchar exige tamaño numérico",
			EN: "Varchar requires a numeric size",
		},
		"mgp_char_size": {
			PT: "Char exige tamanho",
			ES: "Char exige tamaño",
			EN: "Char requires a size",
		},
		"mgp_char_size_numeric": {
			PT: "Char exige tamanho numérico",
			ES: "Char exige tamaño numérico",
			EN: "Char requires a numeric size",
		},
		"mgp_decimal_args": {
			PT: "Decimal exige precisão e escala",
			ES: "Decimal exige precisión y escala",
			EN: "Decimal requires precision and scale",
		},
		"mgp_decimal_numeric": {
			PT: "Decimal exige valores numéricos",
			ES: "Decimal exige valores numéricos",
			EN: "Decimal requires numeric values",
		},
		"mgp_default_value": {
			PT: "Default exige um valor",
			ES: "Default exige un valor",
			EN: "Default requires a value",
		},
		"mgp_default_text": {
			PT: "Default exige valor texto",
			ES: "Default exige valor texto",
			EN: "Default requires a string value",
		},
		"mgp_defaultexpr_value": {
			PT: "DefaultExpr exige um valor",
			ES: "DefaultExpr exige un valor",
			EN: "DefaultExpr requires a value",
		},
		"mgp_defaultexpr_text": {
			PT: "DefaultExpr exige valor texto",
			ES: "DefaultExpr exige valor texto",
			EN: "DefaultExpr requires a string value",
		},
		"mgp_references_args": {
			PT: "References exige tabela e coluna",
			ES: "References exige tabla y columna",
			EN: "References requires a table and a column",
		},
		"mgp_references_text": {
			PT: "References exige parâmetros texto",
			ES: "References exige parámetros texto",
			EN: "References requires string parameters",
		},
		"mgp_constraint_name": {
			PT: "Constraint exige nome",
			ES: "Constraint exige nombre",
			EN: "Constraint requires a name",
		},
		"mgp_constraint_name_text": {
			PT: "Constraint exige nome texto",
			ES: "Constraint exige nombre texto",
			EN: "Constraint requires a string name",
		},
		"mgp_col_method_unsupported": {
			PT: "método de coluna %s não suportado",
			ES: "método de columna %s no soportado",
			EN: "unsupported column method %s",
		},
		"mgp_needs_n_args": {
			PT: "%s exige %d argumento(s)",
			ES: "%s exige %d argumento(s)",
			EN: "%s requires %d argument(s)",
		},

		// ── Catálogo de aliases e views ──
		"cat_read_tables_failed": {
			PT: "ler tabelas de %s: %w",
			ES: "leer las tablas de %s: %w",
			EN: "reading tables from %s: %w",
		},
		"cat_read_aliases_failed": {
			PT: "ler aliases de %s: %w",
			ES: "leer los alias de %s: %w",
			EN: "reading aliases from %s: %w",
		},
		"cat_alias_collision": {
			PT: "os aliases %q e %q geram o mesmo identificador %s no catálogo; escolha apelidos distintos",
			ES: "los alias %q y %q generan el mismo identificador %s en el catálogo; elija apodos distintos",
			EN: "aliases %q and %q generate the same identifier %s in the catalog; choose distinct nicknames",
		},
		"cat_gen_alias_note": {
			PT: "// Identidade FÍSICA de cada tabela, para migration, seeder e factory.\n// ACUMULA e nunca remove: a migration que derrubou uma tabela cita o nome dela,\n// então o nome tem de continuar existindo para ela seguir compilando.\n// A entidade de consulta do mesmo nome (core.Users) fica no entities.gen.go,\n// reflete o schema atual e é OPCIONAL.\n",
			ES: "// Identidad FÍSICA de cada tabla, para migración, seeder y factory.\n// ACUMULA y nunca elimina: la migración que eliminó una tabla cita su nombre,\n// así que el nombre debe seguir existiendo para que ella siga compilando.\n// La entidad de consulta del mismo nombre (core.Users) está en entities.gen.go,\n// refleja el esquema actual y es OPCIONAL.\n",
			EN: "// Each table's PHYSICAL identity, for migration, seeder and factory.\n// ACCUMULATES and never removes: the migration that dropped a table references its\n// name, so the name must keep existing for that migration to still compile.\n// The query entity of the same name (core.Users) lives in entities.gen.go,\n// mirrors the current schema and is OPTIONAL.\n",
		},
		"cat_gen_column_note": {
			PT: "// Identidade das COLUNAS, para migration, seeder e factory. Acumula e nunca remove.\n// Os operadores de consulta da mesma coluna (core.Users.Column.Nome) ficam no entities.gen.go e são OPCIONAIS.\n",
			ES: "// Identidad de las COLUMNAS, para migración, seeder y factory. Acumula y nunca elimina.\n// Los operadores de consulta de la misma columna (core.Users.Column.Nome) están en entities.gen.go y son OPCIONALES.\n",
			EN: "// COLUMN identity, for migration, seeder and factory. Accumulates and never removes.\n// The query operators for the same column (core.Users.Column.Nome) live in entities.gen.go and are OPTIONAL.\n",
		},
		"cat_column_collision": {
			PT: "na tabela %s, as colunas %q e %q geram o mesmo identificador Go (%s) no catalogo; renomeie uma delas na migration",
			ES: "en la tabla %s, las columnas %q y %q generan el mismo identificador Go (%s) en el catalogo; renombre una de ellas en la migracion",
			EN: "in table %s, columns %q and %q generate the same Go identifier (%s) in the catalog; rename one of them in the migration",
		},
		// Colisão no nível da TABELA. A saída é .Alias(), que existe exatamente para
		// isso — ao contrário da coluna, que não tem apelido.
		"cat_table_collision": {
			PT: "as tabelas %q e %q geram o mesmo identificador Go (%s) no catálogo de colunas; dê um apelido a uma delas com .Alias(\"outro_nome\") na migration",
			ES: "las tablas %q y %q generan el mismo identificador Go (%s) en el catálogo de columnas; asigne un alias a una de ellas con .Alias(\"otro_nombre\") en la migración",
			EN: "tables %q and %q generate the same Go identifier (%s) in the column catalog; give one of them a nickname with .Alias(\"another_name\") in the migration",
		},
		"cat_gen_view_note": {
			PT: "// Identidade física de cada view, para as migrations. Acumula e nunca remove.\n",
			ES: "// Identidad física de cada view, para las migraciones. Acumula y nunca elimina.\n",
			EN: "// Each view's physical identity, for migrations. Accumulates and never removes.\n",
		},
	})
}
