package i18n

// Invariante 5: create_table e add_column não podem mentir sobre a forma do objeto.
//
// A mensagem tem de dizer as DUAS formas — a declarada e a do banco —, porque quem lê está
// olhando uma migration que "deveria" ter criado a tabela e vai encontrar outra coisa lá. Sem os
// dois lados, o erro só transfere a dúvida.
func init() {
	Register(Entradas{
		"run_shape_diverged": {
			PT: "a tabela %s já existe no banco com forma DIFERENTE da declarada:\n%s",
			ES: "la tabla %s ya existe en la base con forma DIFERENTE de la declarada:\n%s",
			EN: "table %s already exists in the database with a shape DIFFERENT from the declared one:\n%s",
		},
		"run_shape_missing": {
			PT: "  coluna(s) declarada(s) que o banco NÃO tem: %s",
			ES: "  columna(s) declarada(s) que la base NO tiene: %s",
			EN: "  declared column(s) the database does NOT have: %s",
		},
		"run_shape_type_diff": {
			PT: "coluna %s: o banco tem %s (%s), a migration declara %s (%s)",
			ES: "columna %s: la base tiene %s (%s), la migración declara %s (%s)",
			EN: "column %s: the database has %s (%s), the migration declares %s (%s)",
		},
		// A solução diz para NÃO editar a migration já aplicada, porque é a saída errada que
		// parece certa: editar faz o checksum bater com o arquivo novo e a divergência com o
		// banco fica permanente e invisível.
		"run_shape_diverged_fix": {
			PT: "O objeto no banco não é o que esta migration declara, e aplicar em cima produziria um schema que ninguém declarou.\nSe a tabela do banco está certa, corrija a DECLARAÇÃO com uma migration NOVA (alter_column/add_column) — não edite a migration já aplicada, porque isso só faz o checksum bater e deixa a divergência permanente.\nSe a declaração está certa, ajuste o banco. Em ambiente de teste, resete.\nSe são duas tabelas diferentes com o mesmo nome, dê apelido a uma com CreateTable(\"nome\").Alias(\"outro_nome\").",
			ES: "El objeto en la base no es lo que esta migración declara, y aplicar encima produciría un esquema que nadie declaró.\nSi la tabla de la base está correcta, corrija la DECLARACIÓN con una migración NUEVA (alter_column/add_column) — no edite la migración ya aplicada, porque eso solo hace coincidir el checksum y deja la divergencia permanente.\nSi la declaración está correcta, ajuste la base. En entorno de prueba, resetee.\nSi son dos tablas distintas con el mismo nombre, asigne un alias a una con CreateTable(\"nombre\").Alias(\"otro_nombre\").",
			EN: "The object in the database is not what this migration declares, and applying on top would produce a schema nobody declared.\nIf the database table is right, fix the DECLARATION with a NEW migration (alter_column/add_column) — do not edit the already-applied migration, because that only makes the checksum match and leaves the divergence permanent.\nIf the declaration is right, fix the database. In a test environment, reset it.\nIf these are two different tables sharing a name, give one a nickname with CreateTable(\"name\").Alias(\"another_name\").",
		},
	})
}

// Coluna mais CURTA no banco do que o declarado. Aviso, não erro: quebra em tempo de execução
// e por isso precisa aparecer, mas o leitor promove texto acima de 4000 para Text(), então o
// tamanho legitimamente difere depois de uma volta pelo import.
func init() {
	Register(Entradas{
		"run_shape_narrower": {
			PT: "%s: o banco aceita %d caractere(s) e a migration declara %d",
			ES: "%s: la base acepta %d caracter(es) y la migración declara %d",
			EN: "%s: the database accepts %d character(s) and the migration declares %d",
		},
		"run_shape_narrower_group": {
			PT: "%d coluna(s) são mais CURTAS no banco do que a migration declara. Nada foi alterado — a escrita de um valor no tamanho declarado vai falhar em tempo de execução:",
			ES: "%d columna(s) son más CORTAS en la base de lo que la migración declara. Nada fue alterado — escribir un valor del tamaño declarado fallará en tiempo de ejecución:",
			EN: "%d column(s) are NARROWER in the database than the migration declares. Nothing was changed — writing a value at the declared size will fail at runtime:",
		},
	})
}
