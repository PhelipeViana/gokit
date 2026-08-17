package i18n

// Menu da área ORM. Ele existe porque o reload deixou de gerar entidade: a geração é
// opt-in por AÇÃO, e sem um lugar para pedi-la o recurso ficaria só na linha de comando.
//
// O texto de cada item diz se toca o banco, e como. É a única coisa que separa este menu
// do reload aos olhos de quem escolhe: aqui tudo que envolve banco é `SELECT`.
func init() {
	Register(Entradas{
		"menu_orm": {
			PT: "Área ORM (entidades, catálogos e objetos especiais)",
			ES: "Área ORM (entidades, catálogos y objetos especiales)",
			EN: "ORM area (entities, catalogs and special objects)",
		},
		"tui_orm_options": {
			PT: "Área ORM — nenhuma opção escreve no banco:",
			ES: "Área ORM — ninguna opción escribe en la base:",
			EN: "ORM area — no option writes to the database:",
		},
		"orm_menu_all": {
			PT: "Tudo (catálogos + entidades + objetos especiais)",
			ES: "Todo (catálogos + entidades + objetos especiales)",
			EN: "Everything (catalogs + entities + special objects)",
		},
		"orm_menu_entities": {
			PT: "Colunas, tabelas e entidades (só o corpus, sem banco)",
			ES: "Columnas, tablas y entidades (solo el corpus, sin base)",
			EN: "Columns, tables and entities (corpus only, no database)",
		},
		"orm_menu_special_preview": {
			PT: "Special Mapper — prévia (mostra o que faria)",
			ES: "Special Mapper — vista previa (muestra qué haría)",
			EN: "Special Mapper — preview (shows what it would do)",
		},
		"orm_menu_special_apply": {
			PT: "Special Mapper — aplicar (grava registro e acessadores)",
			ES: "Special Mapper — aplicar (graba registro y accesores)",
			EN: "Special Mapper — apply (writes registry and accessors)",
		},
		"orm_menu_services": {
			PT: "Atualizar Serviços — indisponível",
			ES: "Actualizar Servicios — no disponible",
			EN: "Update Services — unavailable",
		},
		// Item presente e desabilitado, não ausente: o usuário pediu os quatro, e um
		// item que some do menu vira "onde foi?". Ao escolher, explica o que falta.
		"orm_services_missing": {
			PT: "A geração de serviços ainda não tem forma definida — falta o scaffold que dirá onde o serviço mora e o que ele expõe.\nAs entidades e o `Response` que ele usaria já saem em \"Colunas, tabelas e entidades\".",
			ES: "La generación de servicios aún no tiene forma definida — falta el scaffold que dirá dónde vive el servicio y qué expone.\nLas entidades y el `Response` que usaría ya salen en \"Columnas, tablas y entidades\".",
			EN: "Service generation has no defined shape yet — the scaffold that will say where a service lives and what it exposes is still missing.\nThe entities and the `Response` it would use already come out of \"Columns, tables and entities\".",
		},
		"orm_done_catalogs": {
			PT: "Catálogos de tabelas e colunas atualizados a partir do corpus.",
			ES: "Catálogos de tablas y columnas actualizados a partir del corpus.",
			EN: "Table and column catalogs updated from the corpus.",
		},
		"orm_all_special_failed": {
			PT: "as entidades foram geradas, mas o mapeamento de objetos especiais falhou: %w",
			ES: "las entidades fueron generadas, pero el mapeo de objetos especiales falló: %w",
			EN: "the entities were generated, but mapping the special objects failed: %w",
		},
		"tui_orm_failed": {
			PT: "Falha na área ORM",
			ES: "Fallo en el área ORM",
			EN: "ORM area failed",
		},
	})
}
