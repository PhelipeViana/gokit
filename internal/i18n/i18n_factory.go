package i18n

// Mensagens das factories: validação contra as migrations, execução (limpeza,
// inserção, ressincronização de sequência) e os conselhos que traduzem um erro
// do driver em "o que mudar no Data da factory".
func init() {
	Register(Entradas{
		// ── Validação ──
		"fac_none_found_in": {
			PT: "Nenhuma factory encontrada em ",
			ES: "Ninguna factory encontrada en ",
			EN: "No factory found in ",
		},
		"fac_table_not_created": {
			PT: "%s: a tabela %s não é criada por nenhuma migration",
			ES: "%s: la tabla %s no es creada por ninguna migración",
			EN: "%s: table %s is not created by any migration",
		},
		"fac_column_missing": {
			PT: "%s: a coluna %s não existe em %s",
			ES: "%s: la columna %s no existe en %s",
			EN: "%s: column %s does not exist in %s",
		},
		"fac_problems": {
			PT: "%d problema(s) nas factories:\n  - %s",
			ES: "%d problema(s) en las factories:\n  - %s",
			EN: "%d problem(s) in the factories:\n  - %s",
		},
		"fac_problems_fix": {
			PT: "Rode `gokit factory create <tabela>` para regerar a factory a partir da migration.",
			ES: "Ejecute `gokit factory create <tabla>` para regenerar la factory a partir de la migración.",
			EN: "Run `gokit factory create <table>` to regenerate the factory from the migration.",
		},
		"fac_summary": {
			PT: "  %s %d factory(ies), %d ativa(s), %d linha(s) a gerar\n",
			ES: "  %s %d factory(ies), %d activa(s), %d fila(s) a generar\n",
			EN: "  %s %d factory(ies), %d active, %d row(s) to generate\n",
		},
		"fac_fk_cycle": {
			PT: "  Ciclo de chave estrangeira rompido em: ",
			ES: "  Ciclo de clave foránea roto en: ",
			EN: "  Foreign key cycle broken at: ",
		},

		// ── Execução ──
		"fac_none_active": {
			PT: "Nenhuma factory ativa para executar.",
			ES: "Ninguna factory activa para ejecutar.",
			EN: "No active factory to run.",
		},
		"fac_check_table_failed": {
			PT: "verificar a tabela %s: %w",
			ES: "verificar la tabla %s: %w",
			EN: "checking table %s: %w",
		},
		"fac_skipped_no_table": {
			PT: " ignorada: a tabela não existe no banco",
			ES: " ignorada: la tabla no existe en la base",
			EN: " skipped: the table does not exist in the database",
		},
		// Conferência de valor sem banco: cada uma diz o que o driver diria, antes.
		"fck_null_not_allowed": {
			PT: "a coluna não aceita nulo e a expressão devolveu nulo",
			ES: "la columna no acepta nulo y la expresión devolvió nulo",
			EN: "the column does not accept null and the expression returned null",
		},
		"fck_too_long": {
			PT: "o valor tem %d caractere(s) e a coluna aceita %d",
			ES: "el valor tiene %d carácter(es) y la columna acepta %d",
			EN: "the value has %d character(s) and the column accepts %d",
		},
		"fck_check_violated": {
			PT: "o valor %q não está no CHECK declarado (%s)",
			ES: "el valor %q no está en el CHECK declarado (%s)",
			EN: "value %q is not in the declared CHECK (%s)",
		},
		"fck_duplicate": {
			PT: "o valor %q repete o da linha %d, e a coluna exige valor distinto",
			ES: "el valor %q repite el de la fila %d, y la columna exige valor distinto",
			EN: "value %q repeats the one from row %d, and the column requires a distinct value",
		},
		// A factory começa apagando a tabela. Dado que ela não produziu — de seed,
		// de migration ou da aplicação — não é dela para apagar.
		"fac_skipped_has_rows": {
			PT: " ignorada: a tabela já tem dados (use --force para repovoar)",
			ES: " ignorada: la tabla ya tiene datos (use --force para repoblar)",
			EN: " skipped: the table already has data (use --force to repopulate)",
		},
		"fac_nothing_empty": {
			PT: "nenhuma tabela vazia: as factories só povoam tabela sem dados. Use --force para repovoar.",
			ES: "ninguna tabla vacía: las factories solo poblan tablas sin datos. Use --force para repoblar.",
			EN: "no empty table: factories only populate tables without data. Use --force to repopulate.",
		},
		"fac_check_rows_failed": {
			PT: "verificar se a tabela %s já tem dados: %w",
			ES: "verificar si la tabla %s ya tiene datos: %w",
			EN: "checking whether table %s already has data: %w",
		},
		"fac_no_table_exists": {
			PT: "Nenhuma tabela das factories selecionadas existe no banco.",
			ES: "Ninguna tabla de las factories seleccionadas existe en la base.",
			EN: "None of the selected factories' tables exist in the database.",
		},
		"fac_run_failed": {
			PT: "factory de %s: %w",
			ES: "factory de %s: %w",
			EN: "factory for %s: %w",
		},
		"fac_row_line": {
			PT: "  %s %-42s %d linha(s)\n",
			ES: "  %s %-42s %d fila(s)\n",
			EN: "  %s %-42s %d row(s)\n",
		},
		"fac_total": {
			PT: "\n  %s %d tabela(s), %d linha(s) inserida(s)\n",
			ES: "\n  %s %d tabla(s), %d fila(s) insertada(s)\n",
			EN: "\n  %s %d table(s), %d row(s) inserted\n",
		},
		"fac_missing_for_table": {
			PT: "Não existe factory para a tabela %s.",
			ES: "No existe factory para la tabla %s.",
			EN: "There is no factory for table %s.",
		},
		"fac_missing_for_table_fix": {
			PT: "Rode `gokit factory create %s` para criá-la.",
			ES: "Ejecute `gokit factory create %s` para crearla.",
			EN: "Run `gokit factory create %s` to create it.",
		},
		"fac_truncate_failed": {
			PT: "Não foi possível limpar %s: %v",
			ES: "No se pudo limpiar %s: %v",
			EN: "Could not clear %s: %v",
		},
		"fac_truncate_failed_fix": {
			PT: "Alguma tabela filha fora da seleção referencia estas linhas. Rode as factories sem filtro ou inclua a tabela filha.",
			ES: "Alguna tabla hija fuera de la selección referencia estas filas. Ejecute las factories sin filtro o incluya la tabla hija.",
			EN: "A child table outside the selection references these rows. Run the factories without a filter, or include the child table.",
		},
		"fac_row_failed": {
			PT: "linha %d: %w",
			ES: "fila %d: %w",
			EN: "row %d: %w",
		},
		"fac_identity_check_failed": {
			PT: "verificar a coluna de identidade de %s: %w",
			ES: "verificar la columna de identidad de %s: %w",
			EN: "checking the identity column of %s: %w",
		},
		"fac_sequence_skipped": {
			PT: "  - %s: a sequência não foi ressincronizada; a coluna %s não existe no banco (renomeada por migrate.SQL?)",
			ES: "  - %s: la secuencia no fue resincronizada; la columna %s no existe en la base (¿renombrada por migrate.SQL?)",
			EN: "  - %s: the sequence was not resynced; column %s does not exist in the database (renamed by migrate.SQL?)",
		},
		"fac_sequence_failed": {
			PT: "ressincronizar a sequência de %s.%s: %w",
			ES: "resincronizar la secuencia de %s.%s: %w",
			EN: "resyncing the sequence of %s.%s: %w",
		},

		// ── Vínculo entre tabelas ──
		"fac_link_read_failed": {
			PT: "Não foi possível ler %s.%s para resolver o vínculo: %v",
			ES: "No se pudo leer %s.%s para resolver el vínculo: %v",
			EN: "Could not read %s.%s to resolve the link: %v",
		},
		"fac_link_read_failed_fix": {
			PT: "Confira se a tabela e a coluna da Reference estão escritas como na migration.",
			ES: "Verifique que la tabla y la columna de la Reference estén escritas como en la migración.",
			EN: "Check that the table and column in Reference are spelled as in the migration.",
		},
		"fac_link_parent_empty": {
			PT: "A tabela %s está vazia e o vínculo com %s.%s não pode ser resolvido.",
			ES: "La tabla %s está vacía y el vínculo con %s.%s no puede resolverse.",
			EN: "Table %s is empty and the link to %s.%s cannot be resolved.",
		},
		"fac_link_parent_empty_fix": {
			PT: "Rode a factory de %s antes, ou execute sem filtro para que o gokit ordene sozinho.",
			ES: "Ejecute la factory de %s antes, o ejecute sin filtro para que gokit ordene solo.",
			EN: "Run the %s factory first, or run without a filter so gokit orders them itself.",
		},
		"fac_column_failed": {
			PT: "coluna %s: %w",
			ES: "columna %s: %w",
			EN: "column %s: %w",
		},

		// ── Tradução de erro do driver em conselho ──
		"fac_advice_fk": {
			PT: "A tabela pai não tem a linha referenciada. Use migrate.Reference(\"TABELA_PAI\", \"COLUNA\") nessa coluna em vez de um valor fake.",
			ES: "La tabla padre no tiene la fila referenciada. Use migrate.Reference(\"TABLA_PADRE\", \"COLUMNA\") en esa columna en vez de un valor fake.",
			EN: "The parent table does not have the referenced row. Use migrate.Reference(\"PARENT_TABLE\", \"COLUMN\") on that column instead of a fake value.",
		},
		"fac_advice_check": {
			PT: "O valor gerado não passa no CHECK da coluna. Troque por migrate.FakeChoiceIndex(index, ...) com os valores que o CHECK aceita.",
			ES: "El valor generado no pasa el CHECK de la columna. Cámbielo por migrate.FakeChoiceIndex(index, ...) con los valores que el CHECK acepta.",
			EN: "The generated value does not satisfy the column's CHECK. Switch to migrate.FakeChoiceIndex(index, ...) with the values the CHECK accepts.",
		},
		"fac_advice_too_long": {
			PT: "O valor gerado é maior que a coluna. Passe o tamanho da coluna na função Fake*, por exemplo migrate.FakeUniqueText(index, \"Prefixo\", 30).",
			ES: "El valor generado es más grande que la columna. Pase el tamaño de la columna en la función Fake*, por ejemplo migrate.FakeUniqueText(index, \"Prefijo\", 30).",
			EN: "The generated value is longer than the column. Pass the column size to the Fake* function, e.g. migrate.FakeUniqueText(index, \"Prefix\", 30).",
		},
		"fac_advice_unique": {
			PT: "Duas linhas geraram o mesmo valor numa coluna única. Use a variante por índice, como migrate.FakeUniqueText ou migrate.FakeIntIndex.",
			ES: "Dos filas generaron el mismo valor en una columna única. Use la variante por índice, como migrate.FakeUniqueText o migrate.FakeIntIndex.",
			EN: "Two rows generated the same value in a unique column. Use the index-based variant, such as migrate.FakeUniqueText or migrate.FakeIntIndex.",
		},
		"fac_advice_not_null": {
			PT: "Uma coluna obrigatória ficou de fora do Data da factory.",
			ES: "Una columna obligatoria quedó fuera del Data de la factory.",
			EN: "A required column was left out of the factory's Data.",
		},
		"fac_advice_generic": {
			PT: "Confira o Data da factory contra as colunas declaradas na migration.",
			ES: "Compare el Data de la factory con las columnas declaradas en la migración.",
			EN: "Compare the factory's Data against the columns declared in the migration.",
		},
		"fac_insert_failed": {
			PT: "Falha ao inserir a linha %d de %s: %v\n  Valores: %s",
			ES: "Fallo al insertar la fila %d de %s: %v\n  Valores: %s",
			EN: "Failed to insert row %d of %s: %v\n  Values: %s",
		},

		// ── Criação de factory (scaffold) ──
		"fac_no_tables": {
			PT: "Nenhuma tabela encontrada nas migrations.",
			ES: "Ninguna tabla encontrada en las migraciones.",
			EN: "No table found in the migrations.",
		},
		"fac_no_tables_fix": {
			PT: "Rode `gokit migrate create` para declarar a tabela antes de gerar a factory.",
			ES: "Ejecute `gokit migrate create` para declarar la tabla antes de generar la factory.",
			EN: "Run `gokit migrate create` to declare the table before generating the factory.",
		},
		"fac_table_unknown": {
			PT: "A tabela %s não é criada por nenhuma migration.",
			ES: "La tabla %s no es creada por ninguna migración.",
			EN: "Table %s is not created by any migration.",
		},
		"fac_table_unknown_fix": {
			PT: "Confira o nome ou declare a tabela primeiro com `gokit migrate create`.",
			ES: "Verifique el nombre o declare la tabla primero con `gokit migrate create`.",
			EN: "Check the name, or declare the table first with `gokit migrate create`.",
		},
		"fac_register_failed": {
			PT: "registrar factory gerada: %w",
			ES: "registrar la factory generada: %w",
			EN: "registering the generated factory: %w",
		},
		"fac_create_summary": {
			PT: "\n  %s %d criada(s), %d atualizada(s), %d preservada(s)\n",
			ES: "\n  %s %d creada(s), %d actualizada(s), %d preservada(s)\n",
			EN: "\n  %s %d created, %d updated, %d preserved\n",
		},
		"fac_gen_doc": {
			PT: "// %s gera dados fake para a tabela %s.\n",
			ES: "// %s genera datos fake para la tabla %s.\n",
			EN: "// %s generates fake data for table %s.\n",
		},
	})
}

// Falha ao LER uma factory já escrita. Antes era ignorada, e o custo era o
// `factory create` reescrever o arquivo inteiro por cima do que a pessoa editou.
func init() {
	Register(Entradas{
		"fac_read_failed": {
			PT: "a factory %s já existente não pôde ser lida: %v",
			ES: "la factory %s ya existente no pudo ser leída: %v",
			EN: "the existing factory %s could not be read: %v",
		},
		"fac_read_failed_fix": {
			PT: "Corrija o arquivo antes de gerar de novo — `gokit factory validate` aponta a função. Gerar em cima apagaria as expressões ajustadas à mão.",
			ES: "Corrija el archivo antes de generar de nuevo — `gokit factory validate` señala la función. Generar encima borraría las expresiones ajustadas a mano.",
			EN: "Fix the file before generating again — `gokit factory validate` points at the function. Generating over it would erase hand-tuned expressions.",
		},
	})
}

func init() {
	Register(Entradas{
		"fac_skip_identity_only": {
			PT: "%d tabela(s) sem factory: as únicas colunas são de identidade, e quem as preenche é o banco:",
			ES: "%d tabla(s) sin factory: las únicas columnas son de identidad, y quien las llena es la base:",
			EN: "%d table(s) without a factory: their only columns are identity, and the database fills those:",
		},
	})
}
