package i18n

// Mensagens dos seeders: validação (chave primária obrigatória, ID fixo por
// linha), aplicação com controle de checksum e o esqueleto gerado pelo
// scaffold — inclusive os comentários que vão dentro do arquivo criado.
func init() {
	Register(Entradas{
		// ── Validação ──
		"sed_no_createtable_for": {
			PT: "não existe CreateTable para a tabela %q; a pasta do seed precisa ter o nome físico da tabela",
			ES: "no existe CreateTable para la tabla %q; la carpeta del seed necesita el nombre físico de la tabla",
			EN: "there is no CreateTable for table %q; the seed folder must use the table's physical name",
		},
		"sed_empty": {
			PT: "Seeder() está vazio; remova o arquivo se não há dados",
			ES: "Seeder() está vacío; elimine el archivo si no hay datos",
			EN: "Seeder() is empty; delete the file if there is no data",
		},
		"sed_row_without_id": {
			PT: "linha %d não informa %s; o seed inicial exige ID fixo em todas as linhas",
			ES: "la fila %d no informa %s; el seed inicial exige ID fijo en todas las filas",
			EN: "row %d does not provide %s; the initial seed requires a fixed ID on every row",
		},
		"sed_problems": {
			PT: "%s %d problema(s):\n",
			ES: "%s %d problema(s):\n",
			EN: "%s %d problem(s):\n",
		},
		"sed_invalid": {
			PT: "Há seeders inválidos.",
			ES: "Hay seeders inválidos.",
			EN: "There are invalid seeders.",
		},
		"sed_invalid_fix": {
			PT: "Corrija os arquivos acima e rode de novo.",
			ES: "Corrija los archivos de arriba y ejecute de nuevo.",
			EN: "Fix the files above and run again.",
		},
		"sed_none_found": {
			PT: "Nenhum seeder encontrado.",
			ES: "Ningún seeder encontrado.",
			EN: "No seeder found.",
		},
		"sed_validate_summary": {
			PT: "%s %d seeder(s), %d linha(s)\n",
			ES: "%s %d seeder(s), %d fila(s)\n",
			EN: "%s %d seeder(s), %d row(s)\n",
		},
		"sed_validate_line": {
			PT: "  %s %-34s %4d linha(s)  %s\n",
			ES: "  %s %-34s %4d fila(s)  %s\n",
			EN: "  %s %-34s %4d row(s)  %s\n",
		},

		// ── Aplicação ──
		"sed_create_failed": {
			PT: "criar %s: %w",
			ES: "crear %s: %w",
			EN: "creating %s: %w",
		},
		"sed_changed": {
			PT: "O seeder %s foi alterado depois de aplicado.",
			ES: "El seeder %s fue alterado después de aplicado.",
			EN: "Seeder %s was changed after being applied.",
		},
		"sed_changed_fix": {
			PT: "Seeder aplicado é imutável, como migration. Crie um seeder novo para corrigir: gokit seed create %s",
			ES: "Un seeder aplicado es inmutable, como una migración. Cree un seeder nuevo para corregir: gokit seed create %s",
			EN: "An applied seeder is immutable, like a migration. Create a new seeder to fix it: gokit seed create %s",
		},
		"sed_register_failed": {
			PT: "registrar %s: %w",
			ES: "registrar %s: %w",
			EN: "registering %s: %w",
		},
		"sed_applied_summary": {
			PT: "  %s %d seeder(s) aplicado(s), %d já executado(s)\n",
			ES: "  %s %d seeder(s) aplicado(s), %d ya ejecutado(s)\n",
			EN: "  %s %d seeder(s) applied, %d already run\n",
		},

		// ── Scaffold do seeder ──
		"sed_no_createtable": {
			PT: "Não encontrei um CreateTable para %q.",
			ES: "No encontré un CreateTable para %q.",
			EN: "I could not find a CreateTable for %q.",
		},
		"sed_no_createtable_fix": {
			PT: "O seeder precisa de uma tabela declarada. Confira o nome físico ou o alias.",
			ES: "El seeder necesita una tabla declarada. Verifique el nombre físico o el alias.",
			EN: "The seeder needs a declared table. Check the physical name or the alias.",
		},
		"sed_gen_skeleton": {
			PT: "// Esqueleto gerado: ajuste os valores e duplique a linha conforme precisar.",
			ES: "// Esqueleto generado: ajuste los valores y duplique la fila según necesite.",
			EN: "// Generated skeleton: adjust the values and duplicate the row as needed.",
		},
		"sed_gen_update_note": {
			PT: "// Edição: só linhas com ID fixo (declarado em seeder anterior) podem ser alteradas.\n\t\t// Linha sem o ID é inserção, e o banco gera a chave.",
			ES: "// Edición: solo las filas con ID fijo (declarado en un seeder anterior) pueden alterarse.\n\t\t// Una fila sin el ID es inserción, y la base genera la clave.",
			EN: "// Update: only rows with a fixed ID (declared in an earlier seeder) can be changed.\n\t\t// A row without the ID is an insert, and the database generates the key.",
		},
		"sed_gen_title_initial": {
			PT: "Seed inicial de %s — roda junto com o migrate run.",
			ES: "Seed inicial de %s — corre junto con el migrate run.",
			EN: "Initial seed for %s — runs together with migrate run.",
		},
		"sed_gen_title_update": {
			PT: "Atualização de %s — aplicada por: gokit seed run",
			ES: "Actualización de %s — aplicada por: gokit seed run",
			EN: "Update for %s — applied by: gokit seed run",
		},
		"sed_gen_register_failed": {
			PT: "registrar seeder gerado: %w",
			ES: "registrar el seeder generado: %w",
			EN: "registering the generated seeder: %w",
		},
	})
}

// O seeder é soberano sobre os dados. Substitui o invariante anterior, que recusava
// sobrescrever linha divergente e recusava tabela sem chave primária. Decisão do usuário.
func init() {
	Register(Entradas{
		"sed_no_pk_sovereign": {
			PT: "a tabela %s não tem chave primária: este seeder vai SUBSTITUIR o conteúdo dela por inteiro a cada aplicação — sem chave não há como casar linha declarada com linha existente. Confira o arquivo antes de rodar.",
			ES: "la tabla %s no tiene clave primaria: este seeder va a SUSTITUIR su contenido por completo en cada aplicación — sin clave no hay forma de emparejar la fila declarada con la existente. Revise el archivo antes de ejecutar.",
			EN: "table %s has no primary key: this seeder will REPLACE its entire contents on every application — without a key there is no way to match a declared row to an existing one. Review the file before running.",
		},
		"run_seed_replace_failed": {
			PT: "não foi possível limpar a tabela %s para substituir o conteúdo: %w",
			ES: "no fue posible limpiar la tabla %s para sustituir el contenido: %w",
			EN: "could not clear table %s to replace its contents: %w",
		},
		"run_seed_overwritten": {
			PT: "%d linha(s) SOBRESCRITA(S) pelo seeder. O seeder é soberano sobre os dados, então dado que estava no banco foi substituído — inclusive se tiver vindo da aplicação:",
			ES: "%d fila(s) SOBRESCRITA(S) por el seeder. El seeder es soberano sobre los datos, así que lo que estaba en la base fue sustituido — incluso si vino de la aplicación:",
			EN: "%d row(s) OVERWRITTEN by the seeder. The seeder is sovereign over the data, so what was in the database was replaced — including anything that came from the application:",
		},
		"run_seed_replaced_group": {
			PT: "%d tabela(s) sem chave primária tiveram o conteúdo SUBSTITUÍDO por inteiro:",
			ES: "%d tabla(s) sin clave primaria tuvieron el contenido SUSTITUIDO por completo:",
			EN: "%d table(s) without a primary key had their contents entirely REPLACED:",
		},
	})
}
