package i18n

// Mensagens do Special Mapper: o mapeamento de view, function e procedure.
//
// O vocabulário aqui é deliberado: "mapear", "registrar" e "pendente" — nunca
// "criar" nem "aplicar". O gokit não gerencia esses três objetos; quem os cria é a
// pessoa, no banco. Confundir isso na mensagem confundiria o modelo mental.
func init() {
	Register(Entradas{
		"spc_header": {
			PT: "Mapeando objetos especiais de %s (%s): %d encontrado(s) no banco",
			ES: "Mapeando objetos especiales de %s (%s): %d encontrado(s) en la base",
			EN: "Mapping special objects of %s (%s): %d found in the database",
		},
		"spc_group": {
			PT: "%s (%d):",
			ES: "%s (%d):",
			EN: "%s (%d):",
		},
		"spc_detail_columns": {
			PT: "%d coluna(s) — dá acessador de leitura",
			ES: "%d columna(s) — da acceso de lectura",
			EN: "%d column(s) — yields a read accessor",
		},
		"spc_detail_params": {
			PT: "%d parâmetro(s)",
			ES: "%d parámetro(s)",
			EN: "%d parameter(s)",
		},
		"spc_detail_pending": {
			PT: "falta a definição deste dialeto; existe em %s",
			ES: "falta la definición de este dialecto; existe en %s",
			EN: "the definition for this dialect is missing; it exists in %s",
		},
		"spc_prune_reason": {
			PT: "não existe em banco nenhum e nenhum arquivo tem conteúdo",
			ES: "no existe en ninguna base y ningún archivo tiene contenido",
			EN: "does not exist in any database and no file has content",
		},
		"spc_needs_confirm": {
			PT: "Nada foi escrito. Rode `gokit special --confirm` para gravar o registro.",
			ES: "Nada fue escrito. Ejecute `gokit special --confirm` para grabar el registro.",
			EN: "Nothing was written. Run `gokit special --confirm` to save the record.",
		},
		"spc_done": {
			PT: "%d objeto(s) sincronizado(s) com o banco.",
			ES: "%d objeto(s) sincronizado(s) con la base.",
			EN: "%d object(s) synchronized with the database.",
		},
		"spc_read_partial": {
			PT: "parte do catálogo não pôde ser lida: %v — o que foi lido continua valendo",
			ES: "parte del catálogo no pudo ser leída: %v — lo que fue leído sigue valiendo",
			EN: "part of the catalog could not be read: %v — what was read still counts",
		},
		"spc_no_dialect": {
			PT: "a conexão ativa não declara dialeto",
			ES: "la conexión activa no declara dialecto",
			EN: "the active connection declares no dialect",
		},
		"spc_no_config_fix": {
			PT: "Confira o gokit.json e a conexão ativa antes de mapear.",
			ES: "Revise el gokit.json y la conexión activa antes de mapear.",
			EN: "Check gokit.json and the active connection before mapping.",
		},
		// O conteúdo do .sql que o mapper escreve quando falta a definição do dialeto
		// ativo. É pedido de análise, não definição — e diz de onde copiar, porque é o
		// que a pessoa vai querer saber ao abrir o arquivo.
		"spc_placeholder_title": {
			PT: "%s %s — falta a definição para %s",
			ES: "%s %s — falta la definición para %s",
			EN: "%s %s — the definition for %s is missing",
		},
		"spc_placeholder_body": {
			PT: "Use como referência o arquivo de outro dialeto desta mesma pasta (%s).\n\nEscreva o objeto NO BANCO %s e rode `gokit special --confirm`: o mapper lê do\nbanco e grava a definição aqui. Ele nunca cria nada no banco, e nunca traduz\nSQL de um dialeto para outro — tradução automática de TOP/ROWNUM/LIMIT não é\nconfiável, e o que se grava aqui é histórico.\n\nEnquanto este arquivo tiver só comentário, o objeto conta como PENDENTE.",
			ES: "Use como referencia el archivo de otro dialecto de esta misma carpeta (%s).\n\nEscriba el objeto EN LA BASE %s y ejecute `gokit special --confirm`: el mapper lee\nde la base y graba la definición aquí. Nunca crea nada en la base, y nunca traduce\nSQL de un dialecto a otro — la traducción automática de TOP/ROWNUM/LIMIT no es\nconfiable, y lo que se graba aquí es historial.\n\nMientras este archivo solo tenga comentarios, el objeto cuenta como PENDIENTE.",
			EN: "Use the file of another dialect in this same folder as a reference (%s).\n\nWrite the object IN THE %s DATABASE and run `gokit special --confirm`: the mapper\nreads from the database and records the definition here. It never creates anything\nin the database, and never translates SQL between dialects — automatic translation\nof TOP/ROWNUM/LIMIT is not reliable, and what is recorded here is history.\n\nWhile this file holds only comments, the object counts as PENDING.",
		},
	})
}
