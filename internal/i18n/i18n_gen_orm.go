package i18n

// Comentários emitidos no fields.gen.go (mapeamento ORM gerado das migrations).
// O texto sai no idioma do gokit.json no momento da GERAÇÃO — trocar o idioma
// exige regerar. A tag de anotação (TODO/NOTE/...) fica em inglês de propósito:
// é o que IDEs, linters e o todo-tree reconhecem.
func init() {
	Register(Entradas{
		// A 1ª linha do arquivo gerado é SEMPRE a canônica do Go
		// ("// Code generated ... DO NOT EDIT."), que o toolchain reconhece por
		// regex para ignorar arquivos gerados — traduzi-la quebraria golangci-lint,
		// gopls e afins. Esta chave é a EXPLICAÇÃO que vai na linha seguinte.
		"gen_header": {
			PT: "Gerado a partir das migrations pelo GoKit. Alterações aqui são perdidas na próxima geração.",
			ES: "Generado a partir de las migraciones por GoKit. Los cambios aquí se pierden en la próxima generación.",
			EN: "Generated from the migrations by GoKit. Changes here are lost on the next generation.",
		},
		"gen_orm_row": {
			PT: "Linha tipada da entidade (retorno de Get/First). Coluna anulável vira ponteiro.",
			ES: "Fila tipada de la entidad (retorno de Get/First). Columna anulable es puntero.",
			EN: "Typed row of the entity (returned by Get/First). Nullable column becomes a pointer.",
		},
		"gen_orm_row_rel": {
			PT: "Campos de relação: preenchidos só quando pedidos via .With; omitempty esconde no JSON os que não vieram.",
			ES: "Campos de relación: se llenan solo cuando se piden con .With; omitempty los oculta en el JSON.",
			EN: "Relation fields: filled only when requested via .With; omitempty hides the absent ones in JSON.",
		},
		"gen_orm_fieldset": {
			PT: "Operadores por coluna, expostos em orm.<Entidade>.Column.<Coluna>.",
			ES: "Operadores por columna, expuestos en orm.<Entidad>.Field.<Columna>.",
			EN: "Per-column operators, exposed at orm.<Entity>.Column.<Column>.",
		},
		"gen_orm_relations": {
			PT: "Relações da entidade. Cada relação é um método variádico que recebe, em qualquer ordem, relações do destino (aninhamento) e colunas do destino (projeção daquele nó). O aninhamento por parênteses alcança profundidade ilimitada usando só as relações próprias de cada entidade.",
			ES: "Relaciones de la entidad. Cada relación es un método variádico que recibe, en cualquier orden, relaciones del destino (anidamiento) y columnas del destino (proyección de ese nodo). El anidamiento por paréntesis alcanza profundidad ilimitada usando solo las relaciones propias de cada entidad.",
			EN: "Entity relations. Each relation is a variadic method taking, in any order, target relations (nesting) and target columns (projection of that node). Nesting through parentheses reaches unlimited depth using only each entity's own relations.",
		},
		"gen_orm_entity": {
			PT: "Handle único da entidade: embute o Model[Row] (promove Where/Select/OrderBy/Get/... direto na entidade), mais .Column (operadores de banco) e .Relation (relações).",
			ES: "Handle único de la entidad: incrusta el Model[Row] (promueve Where/Select/OrderBy/Get/... directo en la entidad), más .Column (operadores de banco) y .Relation (relaciones).",
			EN: "Single entity handle: embeds Model[Row] (promoting Where/Select/OrderBy/Get/... onto the entity), plus .Column (database operators) and .Relation (relations).",
		},
		"gen_orm_scanner": {
			PT: "Scanner por NOME de coluna — robusto à ordem e à caixa que cada banco devolve.",
			ES: "Scanner por NOMBRE de columna — robusto al orden y a la caja que devuelve cada base.",
			EN: "Scanner keyed by column NAME — robust to the order and casing each database returns.",
		},
		"gen_orm_build": {
			PT: "Construção num func literal para não repetir os literais de Field.",
			ES: "Construcción en un func literal para no repetir los literales de Field.",
			EN: "Built inside a func literal to avoid repeating the Field literals.",
		},
		"gen_orm_init": {
			PT: "O wiring das relações vai num init() (não no literal do var) para evitar ciclo de inicialização entre entidades através dos loaders.",
			ES: "El wiring de las relaciones va en un init() (no en el literal del var) para evitar ciclo de inicialización entre entidades a través de los loaders.",
			EN: "Relation wiring lives in init() (not in the var literal) to avoid an initialization cycle between entities through the loaders.",
		},
		"gen_orm_loader": {
			PT: "Loader tipado da relação: converte os pais, delega ao motor e costura o resultado.",
			ES: "Loader tipado de la relación: convierte los padres, delega al motor y cose el resultado.",
			EN: "Typed relation loader: asserts the parents, delegates to the engine and stitches the result.",
		},
	})
}
