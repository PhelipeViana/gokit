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
		// Leitura parcial não prova ausência. Sem esta recusa, banco fora derruba TODA
		// consulta, o mapa sai vazio e a poda apaga o registro de objeto que existe.
		"spc_absence_unknown": {
			PT: "leitura incompleta: nada será podado nem gerado nesta execução. O que o mapper leu vale; o que ele NÃO viu pode existir, e apagar registro por isso seria perda. Resolva a leitura e rode de novo.",
			ES: "lectura incompleta: nada será podado ni generado en esta ejecución. Lo que el mapper leyó vale; lo que NO vio puede existir, y borrar el registro por eso sería pérdida. Resuelva la lectura y ejecute de nuevo.",
			EN: "incomplete read: nothing will be pruned or generated in this run. What the mapper read counts; what it did NOT see may exist, and deleting the registry over that would be data loss. Fix the read and run again.",
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

// Mensagens do gerador de acessadores de view (passo 5 do mapper).
func init() {
	Register(Entradas{
		"spc_gen_nature": {
			PT: "Acessadores de LEITURA das views mapeadas: Row tipado, operadores por coluna e scanner.\nA view é lida como tabela, então a consulta é a mesma das entidades — mas SEM escrita:\nsó view trivial é atualizável e a regra muda em cada banco, então o handle não a oferece.\nReflete o que o BANCO tem: view derrubada desaparece daqui na próxima passada do mapper.",
			ES: "Accesores de LECTURA de las views mapeadas: Row tipado, operadores por columna y scanner.\nLa view se lee como tabla, así que la consulta es la misma de las entidades — pero SIN escritura:\nsolo la view trivial es actualizable y la regla cambia en cada base, así que el handle no la ofrece.\nRefleja lo que la BASE tiene: una view eliminada desaparece de aquí en la próxima pasada.",
			EN: "READ accessors for the mapped views: typed Row, per-column operators and scanner.\nA view is read like a table, so querying matches the entities — but with NO writes:\nonly trivial views are updatable and the rule differs per database, so the handle omits it.\nMirrors what the DATABASE has: a dropped view disappears from here on the mapper's next pass.",
		},
		"spc_gen_group": {
			PT: "Agrupador das views: core.View.<View>.Where(...).Get(ctx), e core.View.<View>.Column.<Coluna> para os operadores.",
			ES: "Agrupador de las views: core.View.<View>.Where(...).Get(ctx), y core.View.<View>.Column.<Columna> para los operadores.",
			EN: "View grouper: core.View.<View>.Where(...).Get(ctx), and core.View.<View>.Column.<Column> for the operators.",
		},
		"spc_gen_done": {
			PT: "%d view(s) com acessador de leitura em %s — consulte com core.View.<View>",
			ES: "%d view(s) con accesor de lectura en %s — consulte con core.View.<View>",
			EN: "%d view(s) with a read accessor in %s — query them with core.View.<View>",
		},
		"spc_gen_skipped": {
			PT: "%d view(s) sem acessador; elas seguem registradas, só não são consultáveis pelo core:",
			ES: "%d view(s) sin accesor; siguen registradas, solo no son consultables por el core:",
			EN: "%d view(s) without an accessor; they remain recorded, they just are not queryable through core:",
		},
		"spc_view_collision": {
			PT: "%s (colide com %s no identificador %s; view não tem .Alias() para desempatar)",
			ES: "%s (colisiona con %s en el identificador %s; la view no tiene .Alias() para desempatar)",
			EN: "%s (collides with %s on identifier %s; a view has no .Alias() to disambiguate)",
		},
		"spc_gen_invalid": {
			PT: "o acessador de views gerado não é Go válido: %w",
			ES: "el accesor de views generado no es Go válido: %w",
			EN: "the generated view accessor is not valid Go: %w",
		},
	})
}

func init() {
	Register(Entradas{
		"spc_col_impossible": {
			PT: "%s (%d coluna(s) fora do acessador: o nome não vira identificador Go — %s)",
			ES: "%s (%d columna(s) fuera del accesor: el nombre no se vuelve identificador Go — %s)",
			EN: "%s (%d column(s) left out of the accessor: the name is not a valid Go identifier — %s)",
		},
		"spc_no_usable_column": {
			PT: "%s (nenhuma coluna com nome utilizável em Go; o banco numerou as colunas por falta de apelido na definição)",
			ES: "%s (ninguna columna con nombre utilizable en Go; la base numeró las columnas por falta de alias en la definición)",
			EN: "%s (no column with a name usable in Go; the database numbered the columns because the definition gave no aliases)",
		},
	})
}

func init() {
	Register(Entradas{
		"spc_gen_optin": {
			PT: "%d view(s) registradas e nenhuma com acessador. O acessador é por USO, não por inventário:\nliste em gokit.json as views que o código vai ler e rode o mapper de novo.\n\n  \"special\": { \"accessors\": [\"nome_da_view\"] }\n\nGerar para todas custa caro: num legado de 553 views, recompilar o pacote core passa de dez minutos.",
			ES: "%d view(s) registradas y ninguna con accesor. El accesor es por USO, no por inventario:\nliste en gokit.json las views que el código va a leer y ejecute el mapper de nuevo.\n\n  \"special\": { \"accessors\": [\"nombre_de_la_view\"] }\n\nGenerar para todas cuesta caro: en un legado de 553 views, recompilar el paquete core pasa de diez minutos.",
			EN: "%d view(s) recorded and none with an accessor. The accessor follows USE, not inventory:\nlist in gokit.json the views the code will read and run the mapper again.\n\n  \"special\": { \"accessors\": [\"view_name\"] }\n\nGenerating for all of them is expensive: in a 553-view legacy schema, recompiling the core package takes over ten minutes.",
		},
	})
}
