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
		"fcp_func_wrap": {
			PT: "%s(): %w",
			ES: "%s(): %w",
			EN: "%s(): %w",
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
			PT: "Data precisa ser migrate.Fields{...}",
			ES: "Data debe ser migrate.Fields{...}",
			EN: "Data must be migrate.Fields{...}",
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

		// ── Reference e valores ──
		"fcp_seeder_args": {
			PT: "Seeder exige a tabela e ao menos um valor: Seeder(core.Table.Users, 1)",
			ES: "Seeder exige la tabla y al menos un valor: Seeder(core.Table.Users, 1)",
			EN: "Seeder requires the table and at least one value: Seeder(core.Table.Users, 1)",
		},
		"fcp_seeder_literal": {
			PT: "os valores do Seeder precisam ser literais (número ou texto)",
			ES: "los valores del Seeder deben ser literales (número o texto)",
			EN: "Seeder values must be literals (number or string)",
		},
		"fcp_link_args": {
			PT: "Reference recebe a tabela e, opcionalmente, a coluna do pai",
			ES: "Reference recibe la tabla y, opcionalmente, la columna del padre",
			EN: "Reference takes the table and, optionally, the parent column",
		},
		"fcp_link_table_quoted": {
			PT: "o primeiro argumento de Reference precisa ser o nome da tabela entre aspas",
			ES: "el primer argumento de Reference debe ser el nombre de la tabla entre comillas",
			EN: "Reference's first argument must be the table name, quoted",
		},
		"fcp_link_column_quoted": {
			PT: "o segundo argumento de Reference precisa ser o nome da coluna entre aspas",
			ES: "el segundo argumento de Reference debe ser el nombre de la columna entre comillas",
			EN: "Reference's second argument must be the column name, quoted",
		},
		"fcp_literal_unsupported": {
			PT: "literal não suportado: %s",
			ES: "literal no soportado: %s",
			EN: "unsupported literal: %s",
		},
		"fcp_ident_unsupported": {
			PT: "%s não existe aqui; use um literal ou uma função migrate.Fake*",
			ES: "%s no existe aquí; use un literal o una función migrate.Fake*",
			EN: "%s does not exist here; use a literal or a migrate.Fake* function",
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
		"fcp_call_unknown": {
			PT: "chamada não reconhecida",
			ES: "llamada no reconocida",
			EN: "unrecognized call",
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

		// ── Aridade das funções Fake* ──
		"fcp_needs_values": {
			PT: "exige ao menos um valor",
			ES: "exige al menos un valor",
			EN: "requires at least one value",
		},
		"fcp_needs_size": {
			PT: "exige ao menos o tamanho",
			ES: "exige al menos el tamaño",
			EN: "requires at least a size",
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
