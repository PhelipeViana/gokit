package migraterun

// PASSO 5 do Special Mapper: gerar o acessador das views para o código.
//
// A view é a única das três que a ORM já sabe consumir inteira — ela é LIDA como
// tabela. Então o acessador aqui é o mesmo mecanismo das entidades, com uma diferença
// que é o ponto todo: a fachada é `gokitorm.View[T]`, que não tem escrita. Ver o porquê
// em orm/view.go.
//
// A forma do arquivo gerado:
//
//	core.View.AlgarismoRomano.Where(...).Get(ctx)          consulta tipada
//	core.View.AlgarismoRomano.Column.Codigo.Equal(1)       operador por coluna
//	core.View.AlgarismoRomano.Count(ctx)                   terminal direto
//
// O tipo do agrupador é NOMEADO (`viewsMapeadas`), e não anônimo, porque um anônimo
// obrigaria a declarar a lista de campos duas vezes — na struct e no literal. Com 553
// views isso dobraria um arquivo já grande sem ganho nenhum.

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// Um arquivo por TIPO de objeto especial, e o mapper é dono só destes três.
//
// A separação é de responsabilidade, não de otimização — o Go compila por pacote, então
// dividir arquivo dentro do mesmo pacote não muda tempo de build. O que ela dá é
// fronteira clara: `gokit special` escreve estes três e nada mais; `gokit orm` escreve
// entities.gen.go; o executor escreve table.gen.go e column.gen.go. Ninguém pisa no
// arquivo do outro, e cada um pode ser gerado ou não de forma independente.
//
// É também a base das versões de uso planejadas (Light, Normal, Core, Full): quem não
// quer acessador de view simplesmente não tem o arquivo, e nada mais no pacote muda.
const (
	arquivoDeViews      = "view.gen.go"
	arquivoDeFunctions  = "function.gen.go"
	arquivoDeProcedures = "procedure.gen.go"
)

// GerarAcessadorDeViews escreve o views.gen.go a partir das views mapeadas.
//
// Recebe as views já lidas do banco porque a FORMA de uma view só existe lá — não há
// declaração de onde derivá-la, ao contrário da tabela. É essa a dependência real que
// obriga o acessador a vir depois do mapeamento.
//
// View sem coluna legível não gera acessador: sem as colunas não há Row nem scanner, e
// emitir um handle que não sabe ler nada seria pior que a ausência.
func GerarAcessadorDeViews(root string, state config.ConfigState, views map[string]ObjetoEspecial) (int, []string, error) {
	pasta := pastaDoCoreParaViews(root, state)
	if pasta == "" {
		return 0, nil, nil
	}
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		return 0, nil, err
	}
	destino := filepath.Join(pasta, arquivoDeViews)

	// O catálogo antigo, que também se chamava `view.gen.go`, é aposentado por
	// SOBRESCRITA — mesmo nome, conteúdo novo. Por isso não há remoção dele aqui:
	// apagar antes apagaria a própria saída deste passo.
	//
	// O `views.gen.go` (plural) É removido. Ele foi o nome que este gerador usou por uma
	// versão, antes de as responsabilidades serem separadas em um arquivo por tipo. Os
	// dois declaram `var View` no mesmo pacote, então deixar o antigo no disco quebra a
	// compilação — e o erro apontaria para um arquivo gerado que ninguém escreveu.
	//
	// Falha aqui é FATAL: este apaga OUTRO arquivo, então ninguém depois dele reclama.
	if err := os.Remove(filepath.Join(pasta, "views.gen.go")); err != nil && !os.IsNotExist(err) {
		return 0, nil, i18n.Errf("gen_orm_legacy_leftover", "views.gen.go", err)
	}

	// O identificador vem do MESMO lugar que o das tabelas decide, pelo motivo que já
	// custou caro quatro vezes: derivar por conta própria colide onde o catálogo já
	// resolveu. Ver internal/migraterun/factoryresolve.go.
	identificadores := migrationgo.IdentifierByPhysical(projectRoot(root))

	var semColuna []string
	type geradaView struct {
		identificador string
		naoExportado  string
		fisico        string
		colunas       []acao.ColunaDefinicao
	}
	var geradas []geradaView
	usados := map[string]string{}

	// Acessador é OPT-IN, por consumo.
	//
	// Gerar para todas as views de um legado custa caro e medido: 553 acessadores põem
	// 5,3 MB e 553 instanciações de View[T] no mesmo pacote das 754 entidades, e
	// recompilar esse pacote passa de DEZ MINUTOS — cada edição, e o editor junto.
	//
	// O registro dos .sql continua completo: ele é o inventário e é texto. O que se paga
	// por view é o código Go, então ele acompanha o USO, não o inventário. Quem lê uma
	// view no código a lista em `special.accessors`.
	pedidas := viewsPedidas(state)
	for _, nome := range NomesOrdenadosDeEspeciais(views) {
		if !pedidas[nome] {
			continue
		}
		view := views[nome]
		if len(view.Colunas) == 0 {
			semColuna = append(semColuna, nome)
			continue
		}
		identificador := identificadorDaView(view.Nome, identificadores)
		// Duas views cujo nome colapsa no mesmo identificador não podem virar dois
		// campos de mesmo nome. Aqui não há `.Alias()` para desempatar — view não é
		// declarada —, então a segunda fica de fora, dita em voz alta.
		if anterior, ocupado := usados[identificador]; ocupado {
			semColuna = append(semColuna, i18n.Tf("spc_view_collision", nome, anterior, identificador))
			continue
		}
		usados[identificador] = nome

		colunas := make([]acao.ColunaDefinicao, 0, len(view.Colunas))
		vistos := map[string]bool{}
		var colunasImpossiveis []string
		for _, coluna := range view.Colunas {
			identificadorDaColuna := exportedORMIdentifier(coluna.Nome)
			// Nome que não vira identificador Go é PULADO, não saneado.
			//
			// SQL aceita qualquer coisa entre delimitadores, e view de legado usa:
			// `CPF&MATRICULA_CARGA` tem `&`, e há view cujas colunas se chamam `1`, `2`,
			// `3` — apelido que ninguém deu, então o banco numerou. Nenhum dos dois é
			// campo de struct possível. Sanear inventaria um nome que não existe na view
			// e o scanner, que casa por nome de coluna, não acharia nada.
			//
			// A view continua com acessador para as colunas que dá — perder uma coluna é
			// melhor que perder a view inteira, e o scanner já descarta o que não conhece.
			if !identificadorGoValido(identificadorDaColuna) {
				colunasImpossiveis = append(colunasImpossiveis, coluna.Nome)
				continue
			}
			// Coluna repetida no identificador é descartada, não renomeada: a view é o
			// que o banco tem, e inventar nome aqui esconderia o problema real, que é
			// um apelido ruim na definição da view.
			if vistos[identificadorDaColuna] {
				continue
			}
			vistos[identificadorDaColuna] = true
			colunas = append(colunas, acao.ColunaDefinicao{
				Name:     coluna.Nome,
				Type:     familiaDoDSLParaView(coluna),
				Nullable: coluna.Nulo,
			})
		}
		if len(colunasImpossiveis) > 0 {
			semColuna = append(semColuna, i18n.Tf("spc_col_impossible", nome,
				len(colunasImpossiveis), strings.Join(colunasImpossiveis, ", ")))
		}
		// Filtrada, a view pode ter ficado sem coluna nenhuma — é o caso da que numera
		// as colunas de 1 a N. Aí não há Row nem scanner possível.
		if len(colunas) == 0 {
			usados[identificador] = ""
			delete(usados, identificador)
			semColuna = append(semColuna, i18n.Tf("spc_no_usable_column", nome))
			continue
		}
		geradas = append(geradas, geradaView{
			identificador: identificador,
			naoExportado:  desexportar(identificador),
			fisico:        strings.ToLower(view.Nome),
			colunas:       colunas,
		})
	}

	if len(geradas) == 0 {
		// Sem view mapeada o arquivo é removido, e não deixado velho: um acessador
		// apontando para view que não existe mais é pior que acessador nenhum.
		if err := os.Remove(destino); err != nil && !os.IsNotExist(err) {
			return 0, semColuna, err
		}
		return 0, semColuna, nil
	}

	var corpo strings.Builder
	corpo.WriteString(cabecalhoGerado())
	// Linha por linha: `doc` prefixa `//` uma vez só, e a natureza do arquivo é um
	// texto de várias linhas — sem quebrar aqui, a segunda linha sai como código.
	for _, linha := range strings.Split(i18n.T("spc_gen_nature"), "\n") {
		corpo.WriteString("// " + linha + "\n")
	}
	corpo.WriteString("package core\n\n")

	precisaDeTime := false
	for _, gerada := range geradas {
		for _, coluna := range gerada.colunas {
			if strings.Contains(rowGoType(coluna), "time.Time") {
				precisaDeTime = true
			}
		}
	}
	corpo.WriteString("import (\n\t\"database/sql\"\n\t\"strings\"\n")
	if precisaDeTime {
		corpo.WriteString("\t\"time\"\n")
	}
	corpo.WriteString("\n\tgokitorm \"github.com/PhelipeViana/gokit/orm\"\n)\n\n")

	for _, gerada := range geradas {
		escreverTipoDaView(&corpo, gerada.identificador, gerada.naoExportado, gerada.fisico, gerada.colunas)
	}

	// O agrupador: tipo nomeado, depois o valor.
	corpo.WriteString(doc("spc_gen_group"))
	corpo.WriteString("type viewsMapeadas struct {\n")
	for _, gerada := range geradas {
		fmt.Fprintf(&corpo, "\t%s %sView\n", gerada.identificador, gerada.naoExportado)
	}
	corpo.WriteString("}\n\nvar View = viewsMapeadas{\n")
	for _, gerada := range geradas {
		fmt.Fprintf(&corpo, "\t%s: nova%sView(),\n", gerada.identificador, gerada.identificador)
	}
	corpo.WriteString("}\n")

	formatado, err := format.Source([]byte(corpo.String()))
	if err != nil {
		return 0, semColuna, i18n.Errf("spc_gen_invalid", err)
	}
	// Apaga antes de escrever para vencer arquivo somente-leitura; o WriteFile abaixo
	// é quem reporta a falha real.
	_ = os.Remove(destino)
	if err := os.WriteFile(destino, formatado, 0o644); err != nil {
		return 0, semColuna, err
	}
	return len(geradas), semColuna, nil
}

// escreverTipoDaView emite Row, ColumnSet, scanner e construtor de uma view.
func escreverTipoDaView(corpo *strings.Builder, identificador, naoExportado, fisico string, colunas []acao.ColunaDefinicao) {
	// Row tipado. Coluna anulável vira ponteiro, igual às entidades — e em view isso é
	// mais comum, porque LEFT JOIN produz nulo em coluna que na tabela é obrigatória.
	fmt.Fprintf(corpo, "type %sRow struct {\n", identificador)
	for _, coluna := range colunas {
		// A tag JSON vai em MINÚSCULAS, e o nome SQL fica como o banco reporta.
		//
		// São coisas diferentes: o nome SQL entra no SELECT e tem de ser exatamente o
		// que existe na view — o SQL Server devolve o apelido em maiúsculas, e mudá-lo
		// quebraria a consulta. A tag é contrato de API, e ali `{"codigo": 1}` é o que
		// se espera; deixar `{"CODIGO": 1}` faria a resposta de uma view diferir da
		// resposta de uma tabela do mesmo banco, sem nenhum motivo.
		fmt.Fprintf(corpo, "\t%s %s `json:%q`\n",
			exportedORMIdentifier(coluna.Name), rowGoType(coluna), strings.ToLower(coluna.Name))
	}
	corpo.WriteString("}\n\n")

	// Operadores por coluna.
	fmt.Fprintf(corpo, "type %sColumnSet struct {\n", naoExportado)
	for _, coluna := range colunas {
		fmt.Fprintf(corpo, "\t%s %s\n", exportedORMIdentifier(coluna.Name), filterType(coluna.Type))
	}
	corpo.WriteString("}\n\n")

	// Scanner por NOME de coluna, não por posição: cada banco devolve a ordem e a
	// caixa que quer, e view de legado tem apelido em maiúsculas.
	fmt.Fprintf(corpo, "func scan%s(rows *sql.Rows) ([]%sRow, error) {\n", identificador, identificador)
	corpo.WriteString("\tcols, err := rows.Columns()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(corpo, "\tvar out []%sRow\n\tfor rows.Next() {\n", identificador)
	fmt.Fprintf(corpo, "\t\tvar row %sRow\n", identificador)
	corpo.WriteString("\t\tdest := make([]any, len(cols))\n\t\tfor i, c := range cols {\n\t\t\tswitch strings.ToLower(c) {\n")
	for _, coluna := range colunas {
		fmt.Fprintf(corpo, "\t\t\tcase %q:\n\t\t\t\tdest[i] = &row.%s\n",
			strings.ToLower(coluna.Name), exportedORMIdentifier(coluna.Name))
	}
	corpo.WriteString("\t\t\tdefault:\n\t\t\t\tvar descarte any\n\t\t\t\tdest[i] = &descarte\n\t\t\t}\n\t\t}\n")
	corpo.WriteString("\t\tif err := rows.Scan(dest...); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, row)\n\t}\n\treturn out, rows.Err()\n}\n\n")

	// O handle: fachada de leitura + operadores.
	fmt.Fprintf(corpo, "type %sView struct {\n\tgokitorm.View[%sRow]\n\tColumn %sColumnSet\n}\n\n",
		naoExportado, identificador, naoExportado)

	fmt.Fprintf(corpo, "func nova%sView() %sView {\n", identificador, naoExportado)
	for _, coluna := range colunas {
		fmt.Fprintf(corpo, "\t%s := gokitorm.Field{Name: %q, Entity: %q, Table: %q, Column: %q, DataType: %q, Nullable: %t}\n",
			localFieldVar(coluna.Name), exportedORMIdentifier(coluna.Name), fisico, fisico, coluna.Name, coluna.Type, coluna.Nullable)
	}
	fmt.Fprintf(corpo, "\treturn %sView{\n", naoExportado)
	fmt.Fprintf(corpo, "\t\tView: gokitorm.NewView[%sRow](gokitorm.EntityFields{Name: %q, Fields: []gokitorm.Field{", identificador, fisico)
	for indice, coluna := range colunas {
		if indice > 0 {
			corpo.WriteString(", ")
		}
		corpo.WriteString(localFieldVar(coluna.Name))
	}
	fmt.Fprintf(corpo, "}}, scan%s),\n", identificador)
	fmt.Fprintf(corpo, "\t\tColumn: %sColumnSet{\n", naoExportado)
	for _, coluna := range colunas {
		fmt.Fprintf(corpo, "\t\t\t%s: %s(%s),\n",
			exportedORMIdentifier(coluna.Name), filterConstructor(coluna.Type), localFieldVar(coluna.Name))
	}
	corpo.WriteString("\t\t},\n\t}\n}\n\n")
}

// identificadorDaView devolve o identificador do agrupador.
//
// Consulta o catálogo de tabelas primeiro por um motivo prático: se uma view e uma
// tabela tiverem o mesmo nome físico, usar o mesmo identificador mantém as duas
// coerentes. Fora disso, deriva.
func identificadorDaView(nome string, doCatalogo map[string]string) string {
	if identificador, tem := doCatalogo[strings.ToLower(nome)]; tem {
		return identificador
	}
	return migrationgo.ExportedIdentifier(nome)
}

// familiaDoDSLParaView traduz o tipo cru da coluna da view para a família do DSL, que é
// o que rowGoType e filterType entendem.
//
// Reaproveita familiaDoTipo, a mesma tradução usada na leitura de tabela — assim uma
// coluna de view e a coluna de tabela de onde ela vem chegam ao mesmo tipo Go. Tipo que
// o DSL não representa cai em texto, que é o que os quatro bancos aceitam.
func familiaDoDSLParaView(coluna ColunaDoBanco) string {
	switch familiaDoTipo(coluna.Tipo, "") {
	case "inteiro":
		return "integer"
	case "numero":
		return "decimal"
	case "booleano":
		return "boolean"
	case "data":
		return "date"
	case "datahora", "timestamp":
		return "timestamp"
	default:
		// texto, char, texto_longo, binario, guid e o que o catálogo não souber. Todos
		// leem como string em Go, que é o denominador que os quatro bancos aceitam.
		return "string"
	}
}

// pastaDoCoreParaViews devolve a pasta do pacote core.
func pastaDoCoreParaViews(root string, state config.ConfigState) string {
	saida := ""
	if state.Config != nil {
		saida = strings.TrimSpace(state.Config.Output.ORM)
	}
	if saida == "" {
		saida = "internal/gokit/core"
	}
	if filepath.IsAbs(saida) {
		return saida
	}
	return filepath.Join(root, filepath.FromSlash(saida))
}

// identificadorGoValido diz se o texto pode ser nome de campo de struct em Go.
//
// A regra é a da linguagem: começa com letra ou `_`, e o resto é letra, dígito ou `_`.
// Vazio não serve. É a última barreira antes de o gerador emitir algo que não compila —
// e ela é necessária porque SQL aceita nome que Go não aceita.
func identificadorGoValido(texto string) bool {
	if texto == "" {
		return false
	}
	for posicao := 0; posicao < len(texto); posicao++ {
		c := texto[posicao]
		letra := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
		digito := c >= '0' && c <= '9'
		if posicao == 0 && !letra {
			return false
		}
		if !letra && !digito {
			return false
		}
	}
	return true
}

// viewsPedidas devolve, em minúsculas, as views que o projeto pediu acessador.
//
// Nome é comparado sem caixa porque cada banco reporta a view na caixa dele — o SQL
// Server em maiúsculas, o Postgres em minúsculas — e quem escreve o gokit.json não
// deveria ter de saber disso.
func viewsPedidas(state config.ConfigState) map[string]bool {
	pedidas := map[string]bool{}
	if state.Config == nil {
		return pedidas
	}
	for _, nome := range state.Config.Special.Accessors {
		if limpo := strings.ToLower(strings.TrimSpace(nome)); limpo != "" {
			pedidas[limpo] = true
		}
	}
	return pedidas
}
