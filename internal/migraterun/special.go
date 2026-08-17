package migraterun

// Leitura dos OBJETOS ESPECIAIS do banco: view, function e procedure.
//
// São "especiais" no sentido de que o gokit não os declara nem os aplica — ele os
// MAPEIA. O banco é a verdade; aqui só se lê o que existe. Quem cria é a pessoa, no
// banco, e é por isso que nada neste arquivo escreve DDL.
//
// Por que os três juntos e não um arquivo cada: o ciclo é o mesmo — descobrir, registrar
// a definição por dialeto, gerar o acessador — e o que muda entre eles é só o que o
// catálogo sabe contar:
//
//	view       nome + definição + COLUNAS      → dá acessador de leitura completo
//	function   nome + definição + PARÂMETROS   → dá chamada tipada
//	procedure  nome + definição + PARÂMETROS   → dá execução tipada; o RETORNO não é
//	                                              descritível: nenhum catálogo diz o
//	                                              formato do result set de uma procedure
//
// A prioridade medida é sqlserver → oracle. As consultas dos outros dois seguem a mesma
// forma e estão aqui para não deixar o comando quebrar neles.

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

// Os três tipos de objeto especial. São string porque viram nome de pasta e entram no
// monitor: acrescentar um tipo não pode invalidar registro já gravado.
const (
	EspecialView      = "view"
	EspecialFunction  = "function"
	EspecialProcedure = "procedure"
)

// ParametroEspecial é um argumento de function ou procedure.
type ParametroEspecial struct {
	Nome  string
	Tipo  string // tipo cru do banco, em minúsculas
	Ordem int
	// Saida marca OUT e INOUT. Importa para o acessador: parâmetro de saída não é
	// argumento da chamada em Go, é retorno.
	Saida bool
}

// ObjetoEspecial é uma view, function ou procedure lida do banco.
type ObjetoEspecial struct {
	Nome string
	Tipo string
	// SQL é a definição como o banco a guarda, sem o cabeçalho CREATE.
	SQL string
	// Colunas só existe para view.
	Colunas []ColunaDoBanco
	// Parametros só existe para function e procedure, em ordem de declaração.
	Parametros []ParametroEspecial
	// Retorno é o tipo devolvido por function escalar. Vazio em procedure e em
	// function que devolve tabela.
	Retorno string
}

// LerObjetosEspeciais devolve view, function e procedure do schema, indexados por
// tipo e nome em minúsculas.
//
// Falha na leitura de UM tipo não derruba os outros: num banco onde o usuário não tem
// permissão sobre o catálogo de rotinas, ainda vale mapear as views.
func LerObjetosEspeciais(ctx context.Context, db *sql.DB, dialect, schema string) (map[string]map[string]ObjetoEspecial, []error) {
	resultado := map[string]map[string]ObjetoEspecial{
		EspecialView:      {},
		EspecialFunction:  {},
		EspecialProcedure: {},
	}
	var problemas []error

	views, err := LerViewsDoBanco(ctx, db, dialect, schema)
	if err != nil {
		problemas = append(problemas, err)
	} else {
		colunas, err := lerColunasDeViews(ctx, db, dialect, schema)
		if err != nil {
			// Sem as colunas a view ainda é registrável; só o acessador tipado fica de
			// fora. Registrar o problema e seguir é melhor que perder o inventário.
			problemas = append(problemas, err)
		}
		for nome, view := range views {
			resultado[EspecialView][nome] = ObjetoEspecial{
				Nome:    view.Nome,
				Tipo:    EspecialView,
				SQL:     view.SQL,
				Colunas: colunas[nome],
			}
		}
	}

	rotinas, err := lerRotinas(ctx, db, dialect, schema)
	if err != nil {
		problemas = append(problemas, err)
		return resultado, problemas
	}
	parametros, err := lerParametrosDeRotinas(ctx, db, dialect, schema)
	if err != nil {
		problemas = append(problemas, err)
	}
	for chave, rotina := range rotinas {
		rotina.Parametros = parametros[chave]
		resultado[rotina.Tipo][strings.ToLower(rotina.Nome)] = rotina
	}
	return resultado, problemas
}

// lerColunasDeViews devolve as colunas de cada view, indexadas pelo nome da view em
// minúsculas.
//
// É uma consulta SEPARADA da de tabelas porque aquela filtra `table_type = 'BASE
// TABLE'` de propósito — misturar as duas faria o import oferecer migration para view.
// Aqui é o contrário: só view interessa.
func lerColunasDeViews(ctx context.Context, db *sql.DB, dialect, schema string) (map[string][]ColunaDoBanco, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable
			FROM information_schema.columns c
			JOIN information_schema.views v
			  ON v.table_schema = c.table_schema AND v.table_name = c.table_name
			WHERE c.table_schema = $1
			ORDER BY c.table_name, c.ordinal_position`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable
			FROM information_schema.columns c
			JOIN information_schema.views v
			  ON v.table_schema = c.table_schema AND v.table_name = c.table_name
			WHERE c.table_schema = DATABASE()
			ORDER BY c.table_name, c.ordinal_position`

	case "sqlserver":
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			COALESCE(c.character_maximum_length, 0), COALESCE(c.numeric_precision, 0),
			COALESCE(c.numeric_scale, 0), c.is_nullable
			FROM information_schema.columns c
			JOIN information_schema.views v
			  ON v.table_schema = c.table_schema AND v.table_name = c.table_name
			WHERE c.table_schema = @p1
			ORDER BY c.table_name, c.ordinal_position`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		// No Oracle a coluna de view vive em all_tab_columns junto com a de tabela; o
		// que separa é o join com all_views.
		comando = `SELECT c.table_name, c.column_name, c.data_type,
			NVL(c.char_length, 0), NVL(c.data_precision, 0), NVL(c.data_scale, 0), c.nullable
			FROM all_tab_columns c
			JOIN all_views v ON v.owner = c.owner AND v.view_name = c.table_name
			WHERE c.owner = :1
			ORDER BY c.table_name, c.column_id`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	porView := map[string][]ColunaDoBanco{}
	for rows.Next() {
		var view, coluna, tipo, nulo string
		var tamanho, precisao, escala int
		if err := rows.Scan(&view, &coluna, &tipo, &tamanho, &precisao, &escala, &nulo); err != nil {
			return nil, err
		}
		chave := strings.ToLower(view)
		porView[chave] = append(porView[chave], ColunaDoBanco{
			Nome:     coluna,
			Tipo:     strings.ToLower(tipo),
			Tamanho:  tamanho,
			Precisao: precisao,
			Escala:   escala,
			Nulo:     nulo == "YES" || nulo == "Y",
		})
	}
	return porView, rows.Err()
}

// lerRotinas devolve function e procedure com a definição, indexadas por
// tipo:nome em minúsculas — a chave composta existe porque nada impede uma function e
// uma procedure de terem o mesmo nome.
func lerRotinas(ctx context.Context, db *sql.DB, dialect, schema string) (map[string]ObjetoEspecial, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		// prokind: 'f' função, 'p' procedure, 'a' agregação, 'w' window. As duas
		// últimas ficam de fora: não são chamáveis como rotina comum.
		comando = `SELECT p.proname,
			CASE WHEN p.prokind = 'p' THEN 'procedure' ELSE 'function' END,
			COALESCE(pg_get_functiondef(p.oid), ''),
			COALESCE(pg_catalog.format_type(p.prorettype, NULL), '')
			FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = $1 AND p.prokind IN ('f', 'p')
			ORDER BY p.proname`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT routine_name,
			CASE WHEN routine_type = 'PROCEDURE' THEN 'procedure' ELSE 'function' END,
			COALESCE(routine_definition, ''), COALESCE(dtd_identifier, '')
			FROM information_schema.routines
			WHERE routine_schema = DATABASE()
			ORDER BY routine_name`

	case "sqlserver":
		// sys.sql_modules pelo mesmo motivo das views: information_schema trunca a
		// definição em 4000 caracteres, e procedure de legado passa disso com folga.
		//
		// FN escalar, IF inline table-valued, TF multi-statement table-valued, P
		// procedure. O tipo de retorno da escalar vem de sys.types.
		comando = `SELECT o.name,
			CASE WHEN o.type = 'P' THEN 'procedure' ELSE 'function' END,
			COALESCE(m.definition, ''),
			COALESCE(t.name, '')
			FROM sys.objects o
			JOIN sys.sql_modules m ON m.object_id = o.object_id
			JOIN sys.schemas s ON s.schema_id = o.schema_id
			LEFT JOIN sys.parameters p ON p.object_id = o.object_id AND p.parameter_id = 0
			LEFT JOIN sys.types t ON t.user_type_id = p.user_type_id
			WHERE s.name = @p1 AND o.type IN ('P', 'FN', 'IF', 'TF')
			ORDER BY o.name`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		// all_source vem em LINHAS; a definição é a concatenação na ordem de `line`.
		// LISTAGG estoura em 4000 caracteres, então a junção acontece em Go.
		comando = `SELECT o.object_name,
			CASE WHEN o.object_type = 'PROCEDURE' THEN 'procedure' ELSE 'function' END,
			s.text, ''
			FROM all_objects o
			JOIN all_source s ON s.owner = o.owner AND s.name = o.object_name AND s.type = o.object_type
			WHERE o.owner = :1 AND o.object_type IN ('PROCEDURE', 'FUNCTION')
			ORDER BY o.object_name, s.line`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rotinas := map[string]ObjetoEspecial{}
	for rows.Next() {
		var nome, tipo string
		var definicao, retorno sql.NullString
		if err := rows.Scan(&nome, &tipo, &definicao, &retorno); err != nil {
			return nil, err
		}
		if objetoDeSistema(dialect, nome) {
			continue
		}
		chave := tipo + ":" + strings.ToLower(nome)
		atual, existia := rotinas[chave]
		if !existia {
			atual = ObjetoEspecial{Nome: nome, Tipo: tipo, Retorno: strings.ToLower(retorno.String)}
		}
		// O Oracle devolve uma linha por linha do fonte; os outros três devolvem a
		// definição inteira numa linha só. Concatenar cobre os dois casos.
		atual.SQL += definicao.String
		rotinas[chave] = atual
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for chave, rotina := range rotinas {
		rotina.SQL = normalizaDefinicaoDeRotina(rotina.SQL)
		rotinas[chave] = rotina
	}
	return rotinas, nil
}

// lerParametrosDeRotinas devolve os parâmetros por tipo:nome da rotina.
func lerParametrosDeRotinas(ctx context.Context, db *sql.DB, dialect, schema string) (map[string][]ParametroEspecial, error) {
	var comando string
	var argumentos []any

	switch dialect {
	case "postgres":
		comando = `SELECT r.routine_name,
			CASE WHEN r.routine_type = 'PROCEDURE' THEN 'procedure' ELSE 'function' END,
			COALESCE(p.parameter_name, ''), COALESCE(p.data_type, ''), p.ordinal_position,
			COALESCE(p.parameter_mode, 'IN')
			FROM information_schema.parameters p
			JOIN information_schema.routines r
			  ON r.specific_name = p.specific_name AND r.specific_schema = p.specific_schema
			WHERE p.specific_schema = $1
			ORDER BY r.routine_name, p.ordinal_position`
		argumentos = []any{schemaOr(schema, "public")}

	case "mysql":
		comando = `SELECT r.routine_name,
			CASE WHEN r.routine_type = 'PROCEDURE' THEN 'procedure' ELSE 'function' END,
			COALESCE(p.parameter_name, ''), COALESCE(p.data_type, ''), p.ordinal_position,
			COALESCE(p.parameter_mode, 'IN')
			FROM information_schema.parameters p
			JOIN information_schema.routines r
			  ON r.specific_name = p.specific_name AND r.routine_schema = p.specific_schema
			WHERE p.specific_schema = DATABASE() AND p.ordinal_position > 0
			ORDER BY r.routine_name, p.ordinal_position`

	case "sqlserver":
		// parameter_id = 0 é o RETORNO da função escalar, não um argumento.
		comando = `SELECT o.name,
			CASE WHEN o.type = 'P' THEN 'procedure' ELSE 'function' END,
			p.name, t.name, p.parameter_id,
			CASE WHEN p.is_output = 1 THEN 'OUT' ELSE 'IN' END
			FROM sys.parameters p
			JOIN sys.objects o ON o.object_id = p.object_id
			JOIN sys.schemas s ON s.schema_id = o.schema_id
			JOIN sys.types t ON t.user_type_id = p.user_type_id
			WHERE s.name = @p1 AND o.type IN ('P', 'FN', 'IF', 'TF') AND p.parameter_id > 0
			ORDER BY o.name, p.parameter_id`
		argumentos = []any{schemaOr(schema, "dbo")}

	default:
		// position 0 no Oracle é o retorno da função, como o parameter_id 0 do SQL
		// Server. argument_name nulo acontece em retorno e em tipo interno.
		//
		// `package_name IS NULL` restringe a rotina SOLTA — argumento de rotina dentro de
		// package tem outro endereçamento e não é chamável do mesmo jeito. O filtro
		// também é o que mantém a consulta barata: sem ele, o join com all_objects abre
		// para todo argumento de todo package visível ao usuário.
		comando = `SELECT a.object_name,
			CASE WHEN o.object_type = 'PROCEDURE' THEN 'procedure' ELSE 'function' END,
			NVL(a.argument_name, ''), NVL(a.data_type, ''), a.position, NVL(a.in_out, 'IN')
			FROM all_arguments a
			JOIN all_objects o
			  ON o.owner = a.owner
			 AND o.object_name = a.object_name
			 AND o.object_type IN ('PROCEDURE', 'FUNCTION')
			WHERE a.owner = :1 AND a.package_name IS NULL AND a.position > 0
			ORDER BY a.object_name, a.position`
		argumentos = []any{strings.ToUpper(schema)}
	}

	rows, err := db.QueryContext(ctx, comando, argumentos...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	porRotina := map[string][]ParametroEspecial{}
	for rows.Next() {
		var nome, tipo, parametro, tipoDoDado, modo string
		var ordem int
		if err := rows.Scan(&nome, &tipo, &parametro, &tipoDoDado, &ordem, &modo); err != nil {
			return nil, err
		}
		chave := tipo + ":" + strings.ToLower(nome)
		porRotina[chave] = append(porRotina[chave], ParametroEspecial{
			Nome:  strings.TrimPrefix(parametro, "@"),
			Tipo:  strings.ToLower(tipoDoDado),
			Ordem: ordem,
			Saida: strings.Contains(strings.ToUpper(modo), "OUT"),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for chave := range porRotina {
		lista := porRotina[chave]
		sort.SliceStable(lista, func(i, j int) bool { return lista[i].Ordem < lista[j].Ordem })
		porRotina[chave] = lista
	}
	return porRotina, nil
}

// normalizaDefinicaoDeRotina apara a definição sem tentar tirar o cabeçalho.
//
// Diferente da view, aqui o cabeçalho FICA: `CREATE PROCEDURE nome(args) AS ...` é
// inseparável do corpo — a lista de parâmetros faz parte da declaração, e um corpo sem
// assinatura não recria nada. O arquivo guarda a rotina inteira, como ela é no banco.
func normalizaDefinicaoDeRotina(texto string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(texto), ";"))
}

// NomesOrdenadosDeEspeciais devolve os nomes de um tipo em ordem estável.
func NomesOrdenadosDeEspeciais(objetos map[string]ObjetoEspecial) []string {
	nomes := make([]string, 0, len(objetos))
	for nome := range objetos {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)
	return nomes
}
