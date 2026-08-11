package i18n

// Mensagens do parser de factories: a gramática do arquivo de factory lido por
// AST (Table, Ruler, Data) e o vocabulário das funções migrate.Fake*.
func init() {
	Register(Entradas{
		// ── Estrutura da factory ──
		"fcp_column_wrap": {
			PT: "coluna %s: %w",
			ES: "columna %s: %w",
			EN: "column %s: %w",
		},
		"fcp_problems": {
			PT: "%d factory(ies) com problema:\n  - %s",
			ES: "%d factory(ies) con problema:\n  - %s",
			EN: "%d factory(ies) with problems:\n  - %s",
		},
		"fcp_no_func": {
			PT: "nenhuma func ...Factory() migrate.Factory encontrada",
			ES: "ninguna func ...Factory() migrate.Factory encontrada",
			EN: "no func ...Factory() migrate.Factory found",
		},
		"fcp_table_quoted": {
			PT: "Table precisa ser um texto entre aspas",
			ES: "Table debe ser un texto entre comillas",
			EN: "Table must be a quoted string",
		},
		"fcp_no_table": {
			PT: "a factory não declara Table",
			ES: "la factory no declara Table",
			EN: "the factory does not declare Table",
		},
		"fcp_no_data": {
			PT: "a factory não declara Data",
			ES: "la factory no declara Data",
			EN: "the factory does not declare Data",
		},
		"fcp_empty_data": {
			PT: "o Data da factory está vazio",
			ES: "el Data de la factory está vacío",
			EN: "the factory's Data is empty",
		},
		"fcp_ruler_shape": {
			PT: "Ruler precisa ser migrate.Ruler{...}",
			ES: "Ruler debe ser migrate.Ruler{...}",
			EN: "Ruler must be migrate.Ruler{...}",
		},
		"fcp_ruler_count": {
			PT: "Ruler.Count precisa ser um número inteiro",
			ES: "Ruler.Count debe ser un número entero",
			EN: "Ruler.Count must be an integer",
		},
		"fcp_ruler_bool": {
			PT: "Ruler.%s precisa ser true ou false",
			ES: "Ruler.%s debe ser true o false",
			EN: "Ruler.%s must be true or false",
		},
		"fcp_data_signature": {
			PT: "Data precisa ser func(index int) migrate.Fields { return migrate.Fields{...} }",
			ES: "Data debe ser func(index int) migrate.Fields { return migrate.Fields{...} }",
			EN: "Data must be func(index int) migrate.Fields { return migrate.Fields{...} }",
		},
		"fcp_data_needs_index": {
			PT: "Data precisa receber o índice da linha: func(index int) migrate.Fields",
			ES: "Data debe recibir el índice de la fila: func(index int) migrate.Fields",
			EN: "Data must take the row index: func(index int) migrate.Fields",
		},
		"fcp_data_one_return": {
			PT: "o return de Data precisa devolver um único migrate.Fields{...}",
			ES: "el return de Data debe devolver un único migrate.Fields{...}",
			EN: "Data's return must yield a single migrate.Fields{...}",
		},
		"fcp_data_return_shape": {
			PT: "o return de Data precisa ser migrate.Fields{...}",
			ES: "el return de Data debe ser migrate.Fields{...}",
			EN: "Data's return must be migrate.Fields{...}",
		},
		"fcp_data_only_location": {
			PT: "%s: só `nome := migrate.FakeLocation()` e o return são aceitos dentro de Data",
			ES: "%s: solo `nombre := migrate.FakeLocation()` y el return son aceptados dentro de Data",
			EN: "%s: only `name := migrate.FakeLocation()` and the return are accepted inside Data",
		},
		"fcp_data_no_return": {
			PT: "Data não tem return",
			ES: "Data no tiene return",
			EN: "Data has no return",
		},
		"fcp_data_line_shape": {
			PT: "%s: cada linha de Data precisa ser \"COLUNA\": valor",
			ES: "%s: cada línea de Data debe ser \"COLUMNA\": valor",
			EN: "%s: every Data line must be \"COLUMN\": value",
		},
		"fcp_data_column_quoted": {
			PT: "%s: o nome da coluna precisa estar entre aspas",
			ES: "%s: el nombre de la columna debe estar entre comillas",
			EN: "%s: the column name must be quoted",
		},
		"fcp_data_dup_column": {
			PT: "a coluna %s aparece duas vezes em Data",
			ES: "la columna %s aparece dos veces en Data",
			EN: "column %s appears twice in Data",
		},
		"fcp_data_column_wrap": {
			PT: "%s: coluna %s: %w",
			ES: "%s: columna %s: %w",
			EN: "%s: column %s: %w",
		},
		"fcp_data_only_assign": {
			PT: "%s: a única atribuição aceita dentro de Data é `nome := migrate.FakeLocation()`",
			ES: "%s: la única asignación aceptada dentro de Data es `nombre := migrate.FakeLocation()`",
			EN: "%s: the only assignment accepted inside Data is `name := migrate.FakeLocation()`",
		},

		// ── Vinculo e valores ──
		"fcp_link_args": {
			PT: "Vinculo exige a tabela e a coluna: Vinculo(\"TABELA\", \"COLUNA\")",
			ES: "Vinculo exige la tabla y la columna: Vinculo(\"TABLA\", \"COLUMNA\")",
			EN: "Vinculo requires the table and the column: Vinculo(\"TABLE\", \"COLUMN\")",
		},
		"fcp_link_table_quoted": {
			PT: "o primeiro argumento de Vinculo precisa ser o nome da tabela entre aspas",
			ES: "el primer argumento de Vinculo debe ser el nombre de la tabla entre comillas",
			EN: "Vinculo's first argument must be the table name, quoted",
		},
		"fcp_link_column_quoted": {
			PT: "o segundo argumento de Vinculo precisa ser o nome da coluna entre aspas",
			ES: "el segundo argumento de Vinculo debe ser el nombre de la columna entre comillas",
			EN: "Vinculo's second argument must be the column name, quoted",
		},
		"fcp_literal_unsupported": {
			PT: "literal não suportado: %s",
			ES: "literal no soportado: %s",
			EN: "unsupported literal: %s",
		},
		"fcp_ident_unsupported": {
			PT: "%s não existe aqui; use um literal, %s ou uma função migrate.Fake*",
			ES: "%s no existe aquí; use un literal, %s o una función migrate.Fake*",
			EN: "%s does not exist here; use a literal, %s or a migrate.Fake* function",
		},
		"fcp_operator_unsupported": {
			PT: "operador não suportado em valor de factory",
			ES: "operador no soportado en un valor de factory",
			EN: "unsupported operator in a factory value",
		},
		"fcp_negative_numbers_only": {
			PT: "o sinal negativo só vale para números",
			ES: "el signo negativo solo vale para números",
			EN: "the minus sign only applies to numbers",
		},
		"fcp_expr_unsupported": {
			PT: "expressão não suportada; use um literal ou uma função migrate.Fake*",
			ES: "expresión no soportada; use un literal o una función migrate.Fake*",
			EN: "unsupported expression; use a literal or a migrate.Fake* function",
		},
		"fcp_undeclared_location": {
			PT: "%s não foi declarado; use `%s := migrate.FakeLocation()` antes do return",
			ES: "%s no fue declarado; use `%s := migrate.FakeLocation()` antes del return",
			EN: "%s was not declared; use `%s := migrate.FakeLocation()` before the return",
		},
		"fcp_call_unknown": {
			PT: "chamada não reconhecida",
			ES: "llamada no reconocida",
			EN: "unrecognized call",
		},
		"fcp_location_must_bind": {
			PT: "FakeLocation() precisa ser guardada antes do return: `local := migrate.FakeLocation()` e depois `local(%s, \"uf\", 2)`",
			ES: "FakeLocation() debe guardarse antes del return: `local := migrate.FakeLocation()` y luego `local(%s, \"uf\", 2)`",
			EN: "FakeLocation() must be bound before the return: `local := migrate.FakeLocation()` and then `local(%s, \"uf\", 2)`",
		},
		"fcp_not_in_vocabulary_hint": {
			PT: "%s não existe no vocabulário das factories; você quis dizer %s?",
			ES: "%s no existe en el vocabulario de las factories; ¿quiso decir %s?",
			EN: "%s does not exist in the factory vocabulary; did you mean %s?",
		},
		"fcp_not_in_vocabulary": {
			PT: "%s não existe no vocabulário das factories",
			ES: "%s no existe en el vocabulario de las factories",
			EN: "%s does not exist in the factory vocabulary",
		},
		"fcp_location_accessor": {
			PT: "o acessor de localidade exige (índice, campo) e aceita um tamanho: local(index, \"cidade\", 60)",
			ES: "el accesor de localidad exige (índice, campo) y acepta un tamaño: local(index, \"cidade\", 60)",
			EN: "the locality accessor requires (index, field) and accepts a size: local(index, \"cidade\", 60)",
		},

		// ── Aridade das funções Fake* ──
		"fcp_needs_index": {
			PT: "exige ao menos o índice",
			ES: "exige al menos el índice",
			EN: "requires at least the index",
		},
		"fcp_needs_index_size": {
			PT: "exige ao menos índice e tamanho",
			ES: "exige al menos índice y tamaño",
			EN: "requires at least an index and a size",
		},
		"fcp_needs_n_got": {
			PT: "exige %d argumento(s), recebeu %d",
			ES: "exige %d argumento(s), recibió %d",
			EN: "requires %d argument(s), got %d",
		},
		"fcp_arg_int": {
			PT: "o argumento %d precisa ser um número inteiro, veio %T",
			ES: "el argumento %d debe ser un número entero, vino %T",
			EN: "argument %d must be an integer, got %T",
		},
		"fcp_arg_text": {
			PT: "o argumento %d precisa ser um texto entre aspas, veio %T",
			ES: "el argumento %d debe ser un texto entre comillas, vino %T",
			EN: "argument %d must be a quoted string, got %T",
		},
	})
}
