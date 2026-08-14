package migraterun

// Escrita da migration a partir do que existe no banco.
//
// É o caminho inverso do motor, e por isso o mais delicado: migration é histórico
// versionado, então gerar arquivo é ação que não se desfaz sozinha. Por decisão de
// projeto, `import` sem `--confirm` só MOSTRA o que escreveria.

import (
	"context"
	"database/sql"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/aviso"
	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
	"github.com/PhelipeViana/gokit/internal/migrationgo"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// MigrateImport escreve uma migration `create_table` para cada tabela que existe
// no banco e não está declarada no corpus. Sem `confirmar`, apenas mostra.
func MigrateImport(root string, state config.ConfigState, confirmar bool) error {
	// O import do pacote core, para o CreateIndex endereçar a tabela por referência
	// de catálogo — o validador não aceita string ali. Sem módulo resolvido não há
	// referência possível, e nesse caso o índice fica de fora, avisado.
	importCore := ""
	if pRoot := projectRoot(root); pRoot != "" {
		if modulo, err := GetModuleName(pRoot); err == nil {
			importCore = modulo + "/internal/gokit/core"
		}
	}

	tabelas, err := lerEsquemaAtivo(state)
	if err != nil {
		return err
	}

	declaradas, err := tableShapes(root, state)
	if err != nil {
		return err
	}
	noCorpus := map[string]bool{}
	for _, forma := range declaradas {
		noCorpus[strings.ToLower(forma.Table)] = true
	}

	var pendentes []TabelaDoBanco
	for _, nome := range NomesOrdenados(tabelas) {
		if !noCorpus[nome] {
			pendentes = append(pendentes, tabelas[nome])
		}
	}

	viewsPendentes, err := viewsForaDoCorpus(root, state)
	if err != nil {
		return err
	}

	if len(pendentes) == 0 && len(viewsPendentes) == 0 {
		fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.T("scan_in_sync"))
		return nil
	}

	// Pai antes de filha: a ordem de execução das migrations é o timestamp do nome
	// do arquivo, então a ordenação tem de acontecer AQUI, na hora de nomear.
	// Errar isso faz a FK da primeira migration apontar para tabela que ainda não
	// existe, e o erro só aparece no `migrate run`.
	pendentes = ordenaPorDependencia(pendentes, noCorpus)

	fmt.Printf("\n%s\n", i18n.Tf("imp_header", len(pendentes)+len(viewsPendentes)))

	// Um segundo de diferença entre arquivos, na ordem de dependência. Segundo é a
	// resolução do nome do arquivo; sem o passo, 40 tabelas nasceriam com o mesmo
	// timestamp e a ordem entre elas ficaria indefinida.
	// O monitor acompanha a leitura inteira: num banco grande o aviso rola na tela
	// e se perde, e o que o import não carregou é justamente a lista de trabalho.
	monitor := NovoMonitor("migrate import", state.ActiveClient, strings.ToLower(state.ActiveDialect))

	// Os apelidos são escolhidos ANTES de gerar qualquer arquivo: o desempate
	// precisa conhecer o conjunto inteiro para ser determinístico.
	apelidos := aliasesUnicos(pendentes, monitor)

	base := time.Now()
	planejados := make([]arquivoPlanejado, 0, len(pendentes))
	for posicao, tabela := range pendentes {
		conteudo, err := migrationDaTabela(root, state.Config.Output.Migrate, strings.ToLower(state.ActiveDialect), importCore, tabela, base.Add(time.Duration(posicao)*time.Second), monitor, apelidos)
		if err != nil {
			return err
		}
		planejados = append(planejados, conteudo)
		fmt.Printf("  %s %-34s %s\n", cliui.Warning("+"), conteudo.Nome,
			cliui.Muted(i18n.Tf("imp_table_summary", tabela.Nome, len(tabela.Colunas))))
	}

	// O aviso vem depois da lista e antes da confirmação: é o único lugar em que o
	// gokit acrescentou algo que a origem não tinha, e isso não pode ficar só no
	// comentário de um arquivo entre centenas.
	if avisadas := tabelasComIdentidadeForaDaChave(pendentes); len(avisadas) > 0 {
		fmt.Printf("\n%s\n", i18n.Tf("imp_identity_warning", len(avisadas)))
		for _, aviso := range avisadas {
			fmt.Printf("  %s %s\n", cliui.Muted("!"), aviso)
		}
	}
	for _, tabela := range pendentes {
		for _, coluna := range IdentidadeForaDaChave(tabela) {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaIdentidadeForaDaChave,
				Tabela:  strings.ToLower(tabela.Nome),
				Coluna:  coluna,
				Origem:  "identidade sem participar da chave primária",
				Decisao: "declarada fiel; no MySQL o executor cria UNIQUE KEY para a coluna",
			})
		}
		// Índice só é declarável por referência de catálogo, que exige módulo
		// resolvido. Sem ele o índice fica de fora — e ficar de fora em silêncio é
		// exatamente o que o monitor existe para impedir.
		if importCore == "" {
			for _, indice := range tabela.Indices {
				monitor.Registrar(Ocorrencia{
					Tipo:    OcorrenciaNaoLido,
					Tabela:  strings.ToLower(tabela.Nome),
					Objeto:  indice.Nome,
					Origem:  "índice (" + strings.Join(indice.Colunas, ", ") + ")",
					Decisao: "não declarado: o módulo do projeto não foi resolvido, e o índice exige core.Table.*",
				})
			}
		}
	}

	// As views não têm mais ordem a respeitar: elas não são aplicadas pelo migrate, só
	// registradas. Quem cria a view — a pessoa, no banco — resolve a ordem lá.
	viewsPendentes = viewsImportaveis(viewsPendentes, monitor)
	planejadasViews, err := registrarViewsMapeadas(root, state.Config.Output.Migrate, strings.ToLower(state.ActiveDialect), viewsPendentes, monitor)
	if err != nil {
		return err
	}
	for _, planejada := range planejadasViews {
		fmt.Printf("  %s %-34s %s\n", cliui.Warning("+"), planejada.Nome,
			cliui.Muted(i18n.Tf("imp_view_summary", planejada.Tabela, planejada.DialetoDaView)))
	}
	planejados = append(planejados, planejadasViews...)

	if len(planejados) == 0 {
		fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.T("scan_in_sync"))
		return nil
	}

	if !confirmar {
		fmt.Printf("\n%s\n", planejados[0].Conteudo)
		if len(planejados) > 1 {
			fmt.Printf("%s\n", cliui.Muted(i18n.Tf("imp_preview_rest", len(planejados)-1)))
		}
		// A prévia mostra o resumo do monitor mas NÃO grava o arquivo: nada é escrito
		// sem confirmação, e isso inclui o registro.
		if !monitor.Vazio() {
			fmt.Printf("\n%s\n", i18n.Tf("mon_resumo", monitor.Total))
			for _, linha := range monitor.Resumo() {
				fmt.Printf("  %s %s\n", cliui.Muted("·"), linha)
			}
		}
		fmt.Printf("\n%s\n\n", i18n.T("imp_needs_confirm"))
		return nil
	}

	for _, planejado := range planejados {
		// View é REGISTRO, não migration: vem sem caminho de arquivo .go, e só o .sql
		// do dialeto é gravado. O `Caminho` vazio é o que distingue os dois.
		if planejado.Caminho == "" {
			if planejado.SQLDaView == "" {
				continue
			}
			if err := os.MkdirAll(planejado.PastaDaView, 0o755); err != nil {
				return err
			}
			arquivoSQL := filepath.Join(planejado.PastaDaView, planejado.DialetoDaView+".sql")
			if err := os.WriteFile(arquivoSQL, []byte(planejado.SQLDaView+"\n"), 0o644); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(planejado.Caminho), 0o755); err != nil {
			return err
		}
		// Nunca sobrescrever: se o arquivo existe, alguém já importou ou escreveu
		// à mão, e reescrever apagaria decisão humana.
		if _, err := os.Stat(planejado.Caminho); err == nil {
			return cliui.NewUserError(
				i18n.Tf("imp_file_exists", planejado.Nome),
				i18n.T("imp_file_exists_fix"),
			)
		}
		if err := os.WriteFile(planejado.Caminho, []byte(planejado.Conteudo), 0o644); err != nil {
			return err
		}
		if err := recordGeneratedFile(root, planejado.Tabela, "migration", planejado.Caminho); err != nil {
			return i18n.Errf("run_mig_register_failed", err)
		}
	}

	// O catálogo tem de ser reescrito AGORA, e não no próximo `migrate run`: as
	// migrations que acabaram de nascer referenciam core.Table.X no CreateIndex, e o
	// validate confere a referência contra o catálogo em disco. Sem este passo, o
	// import produz um corpus que o próprio validate recusa.
	pRoot := projectRoot(root)
	if pRoot == "" {
		pRoot = root
	}
	// A view entra no catálogo ANTES do RefreshCatalog, porque este relê o corpus —
	// e reler o corpus inclui parsear a migration de view, que exige core.View.X
	// já existente.
	if err := registrarViewsNoCatalogo(pRoot, viewsPendentes); err != nil {
		return i18n.Errf("imp_catalog_failed", err)
	}
	if err := migrationgo.RefreshCatalog(pRoot, pastaDeMigrations(root, state.Config.Output.Migrate)); err != nil {
		return i18n.Errf("imp_catalog_failed", err)
	}

	if caminhoDoMonitor, err := monitor.Escrever(root, state.Config.Output.Docs); err != nil {
		// Falhar aqui não pode invalidar as migrations já gravadas: o registro é
		// informação sobre o import, não parte dele.
		fmt.Printf("  %s %s\n", cliui.Warning("!"), i18n.Tf("mon_falhou", err))
	} else if caminhoDoMonitor != "" {
		fmt.Printf("\n%s\n", i18n.Tf("mon_gravado", monitor.Total, caminhoDoMonitor))
		for _, linha := range monitor.Resumo() {
			fmt.Printf("  %s %s\n", cliui.Muted("·"), linha)
		}
		// O canal recebe o resumo agrupado por nível; o detalhe de cada ocorrência
		// fica no arquivo. Um schema legado gera centenas, e centenas de posts no
		// canal seria o mesmo que nenhum.
		monitor.Notificar(aviso.DoEstado(state), caminhoDoMonitor)
	}

	fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.Tf("imp_written", len(planejados)))
	return nil
}

// tabelasComIdentidadeForaDaChave descreve, em uma linha por tabela, onde o gokit
// precisou acrescentar índice para o MySQL aceitar a coluna auto-incremento.
func tabelasComIdentidadeForaDaChave(tabelas []TabelaDoBanco) []string {
	var avisos []string
	for _, tabela := range tabelas {
		if colunas := IdentidadeForaDaChave(tabela); len(colunas) > 0 {
			avisos = append(avisos, fmt.Sprintf("%s: %s", strings.ToLower(tabela.Nome), strings.Join(colunas, ", ")))
		}
	}
	return avisos
}

type arquivoPlanejado struct {
	Nome     string
	Caminho  string
	Tabela   string
	Conteudo string
	// Os três abaixo só valem para view: o SQL não vai dentro do .go, vai num
	// arquivo .sql na pasta versionada que o parser procura pelo timestamp.
	SQLDaView     string
	PastaDaView   string
	DialetoDaView string
}

// ordenaPorDependencia põe pai antes de filha. FK para tabela que já está no
// corpus não conta: ela será criada por migration anterior.
//
// Ciclo não aborta — schema legado tem ciclo de verdade. A tabela entra na ordem
// em que apareceu, e a FK que fecha o ciclo vai falhar no `migrate run` com um erro
// claro, que é melhor que não importar nada.
func ordenaPorDependencia(tabelas []TabelaDoBanco, noCorpus map[string]bool) []TabelaDoBanco {
	porNome := map[string]TabelaDoBanco{}
	for _, tabela := range tabelas {
		porNome[strings.ToLower(tabela.Nome)] = tabela
	}

	var ordenadas []TabelaDoBanco
	visitada := map[string]bool{}
	naPilha := map[string]bool{}

	var visitar func(chave string)
	visitar = func(chave string) {
		if visitada[chave] || naPilha[chave] {
			return
		}
		naPilha[chave] = true
		tabela := porNome[chave]
		// Pais primeiro, em ordem estável.
		pais := make([]string, 0, len(tabela.ForeignKeys))
		for _, fk := range tabela.ForeignKeys {
			pai := strings.ToLower(fk.TabelaPai)
			if pai == chave || noCorpus[pai] {
				continue
			}
			if _, existe := porNome[pai]; existe {
				pais = append(pais, pai)
			}
		}
		sort.Strings(pais)
		for _, pai := range pais {
			visitar(pai)
		}
		naPilha[chave] = false
		visitada[chave] = true
		ordenadas = append(ordenadas, tabela)
	}

	chaves := make([]string, 0, len(porNome))
	for chave := range porNome {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)
	for _, chave := range chaves {
		visitar(chave)
	}
	return ordenadas
}

// migrationDaTabela monta o arquivo de uma tabela. O nome físico vem do banco em
// minúsculas: o Oracle devolve maiúsculas, e escrever assim no corpus faria o
// MySQL em Linux procurar uma tabela que não existe.
func migrationDaTabela(root, saida, dialect, importCore string, tabela TabelaDoBanco, quando time.Time, monitor *Monitor, apelidos map[string]string) (arquivoPlanejado, error) {
	fisica := strings.ToLower(tabela.Nome)

	chave := map[string]bool{}
	for _, coluna := range tabela.PrimaryKey {
		chave[strings.ToLower(coluna)] = true
	}
	// As colunas de uma FK COMPOSTA vêm em linhas separadas do catálogo, amarradas
	// pelo nome da constraint. Agrupar é o que distingue "duas FKs de uma coluna" de
	// "uma FK de duas colunas" — e as duas coisas exigem integridade diferente.
	porConstraint := map[string][]FKDoBanco{}
	var ordemDasConstraints []string
	for _, fk := range tabela.ForeignKeys {
		nome := strings.ToLower(fk.Constraint)
		if _, visto := porConstraint[nome]; !visto {
			ordemDasConstraints = append(ordemDasConstraints, nome)
		}
		porConstraint[nome] = append(porConstraint[nome], fk)
	}

	// `pai` só recebe a FK de UMA coluna: é a que vira `.References()` na própria
	// coluna. A composta não cabe ali — o DSL da coluna aceita um par só — e sai como
	// operação de tabela, depois do CreateTable.
	pai := map[string]FKDoBanco{}
	var compostas []string
	for _, nome := range ordemDasConstraints {
		partes := porConstraint[nome]
		if len(partes) == 1 {
			pai[strings.ToLower(partes[0].Coluna)] = partes[0]
			continue
		}
		compostas = append(compostas, nome)
	}

	var linhas []string
	for _, coluna := range tabela.Colunas {
		nome := strings.ToLower(coluna.Nome)
		declaracao := "\t\tmigrate.Col(" + fmt.Sprintf("%q", nome) + ")" + declaracaoDaColuna(coluna, dialect)
		registrarDefaultNaoPortavel(monitor, tabela.Nome, coluna)
		// PrimaryKey em cada coluna também na chave composta: o executor detecta
		// mais de uma e emite constraint de tabela, porque PRIMARY KEY inline
		// repetido dá ORA-02260.
		if chave[nome] {
			declaracao += ".PrimaryKey()"
		}
		if coluna.Identity {
			// Nada além disso, mesmo quando a identidade está FORA da chave: o
			// executor já resolve o caso do MySQL sozinho, emitindo uma UNIQUE KEY
			// para a coluna AUTO_INCREMENT que não participa da PK (ver
			// createTableSQL). Acrescentar .Index() aqui produzia um segundo índice
			// redundante sobre a mesma coluna.
			declaracao += ".AutoIncrement()"
		}
		if fk := pai[nome]; fk.TabelaPai != "" {
			declaracao += fmt.Sprintf(".References(%q, %q)",
				strings.ToLower(fk.TabelaPai), strings.ToLower(fk.ColunaPai))
		}
		linhas = append(linhas, declaracao+",")
	}

	// O alias sai da tabela de apelidos, que já garantiu unicidade do identificador.
	// Por padrão ele é o PRÓPRIO nome físico, e não um camelCase dele.
	//
	// O catálogo guarda o alias como VALOR de core.Table.X — `migrate.Table("zzDef")`
	// — e o executor usa esse valor direto como nome de tabela no DDL. Com alias
	// camelCase, o CreateIndex procurava a tabela `zzDef` e o banco só tem `zz_def`.
	// No corpus escrito à mão isso nunca apareceu porque toda tabela de uma palavra
	// tem alias igual ao físico. Aqui, nome com underscore expõe.
	alias := apelidos[fisica]
	if alias == "" {
		alias = fisica
	}
	operacoes := []string{"migrate.CreateTable(" + fmt.Sprintf("%q", fisica) + ",\n" +
		strings.Join(linhas, "\n") + "\n\t).Alias(" + fmt.Sprintf("%q", alias) + ")"}

	// Índice é operação separada, na MESMA migration: o DSL não tem marcador de
	// índice na coluna (o `.Index()` que existia só valia no MySQL e foi removido),
	// e o nome precisa ser explícito porque o Oracle trunca identificador em 30 e o
	// nome é único por schema nele e no SQL Server.
	// O argumento de tabela do CreateIndex precisa ser referência de catálogo: o
	// validador só aceita string no CreateTable, que é quem batiza o nome. O
	// catálogo é reescrito a cada leitura do corpus, então a referência existe
	// assim que a migration de criação existe — mesmo arquivo, inclusive.
	//
	// Sem módulo resolvido não existe referência possível, e o índice fica de fora
	// em vez de gerar um arquivo que o próprio validate recusa.
	// A referência deriva do ALIAS, não do nome físico: é do alias que o catálogo
	// deriva a entrada. `zz_def` vira alias `zzDef`, e o catálogo grava `Zzdef` —
	// enquanto o nome físico daria `ZzDef`, que não existe. Derivar da mesma fonte
	// é o que mantém arquivo e catálogo falando do mesmo símbolo.
	refTabela := ""
	if importCore != "" {
		refTabela = "core.Table." + migrationgo.ExportedIdentifier(alias)
	}

	// FK composta, como operação de tabela. Vem antes dos índices só por leitura: é
	// integridade, e integridade lê-se antes de otimização.
	for _, nome := range compostas {
		partes := porConstraint[nome]
		if refTabela == "" {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Tabela:  fisica,
				Objeto:  nome,
				Origem:  "chave estrangeira composta",
				Decisao: "não declarada: o módulo do projeto não foi resolvido, e a operação exige core.Table.*",
			})
			continue
		}
		mapeamentos := make([]string, 0, len(partes))
		for _, parte := range partes {
			mapeamentos = append(mapeamentos, fmt.Sprintf("%q",
				strings.ToLower(parte.Coluna)+":"+strings.ToLower(parte.ColunaPai)))
		}
		operacoes = append(operacoes, fmt.Sprintf("migrate.AddCompositeForeignKey(%s, %q, %q, %s)",
			refTabela, sanearNomeGerado(strings.ToLower(nome)), strings.ToLower(partes[0].TabelaPai),
			strings.Join(mapeamentos, ", ")))
	}
	// Índice já declarado, por nome+colunas. Schema legado acumula redundância — a
	// mesma coluna com duas unique constraints de nome gerado pelo banco
	// (UQ__MOTIVO_EXCLUSAO__214BF109 e UQ__MOTIVO_EXCLUSAO__6339AFF7) — e, como o nome
	// gerado aqui deriva de tabela+colunas, as duas colapsam no mesmo nome. Declarar
	// duas vezes o mesmo índice não adiciona garantia nenhuma e não compila como DDL.
	jaDeclarados := map[string]bool{}
	for _, indice := range tabela.Indices {
		if refTabela == "" {
			break
		}
		nomeDoIndice := strings.ToLower(indice.Nome)
		if !acao.NomeFisicoValido(nomeDoIndice) {
			// Aqui a decisão depende do que se perde.
			//
			// Índice COMUM é otimização: pular preserva a verdade — sanear inventaria um
			// nome que não existe no banco, e um DropIndex futuro procuraria o objeto
			// errado. O legado tem índices chamados `<IDX_ALGO>`, com os sinais do
			// modelo que alguém copiou.
			//
			// Índice ÚNICO é garantia de integridade, e pular apaga a garantia: o schema
			// migrado aceitaria duplicata que a origem recusa. Entre perder o nome e
			// perder a regra, perde-se o nome. O nome gerado é registrado, para que
			// ninguém procure no banco de destino o nome que estava na origem.
			if !indice.Unico {
				monitor.Registrar(Ocorrencia{
					Tipo:    OcorrenciaNaoLido,
					Tabela:  fisica,
					Objeto:  indice.Nome,
					Origem:  "nome de índice fora da convenção do corpus",
					Decisao: "não declarado: renomeie o índice no banco e importe de novo",
				})
				continue
			}
			gerado := nomeGeradoDeIndiceUnico(fisica, indice.Colunas)
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Tabela:  fisica,
				Objeto:  indice.Nome,
				Origem:  "nome de índice ÚNICO fora da convenção do corpus",
				Decisao: "declarado com o nome gerado " + gerado + ": perder o nome é melhor que perder a unicidade",
			})
			nomeDoIndice = gerado
		}
		chave := nomeDoIndice + "(" + strings.ToLower(strings.Join(indice.Colunas, ",")) + ")"
		if jaDeclarados[chave] {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Tabela:  fisica,
				Objeto:  indice.Nome,
				Origem:  "índice redundante: mesma lista de colunas de um já declarado",
				Decisao: "não declarado de novo; a garantia do primeiro é idêntica",
			})
			continue
		}
		jaDeclarados[chave] = true

		metodo := "migrate.CreateIndex"
		if indice.Unico {
			metodo = "migrate.CreateUniqueIndex"
		}
		colunas := make([]string, 0, len(indice.Colunas))
		for _, coluna := range indice.Colunas {
			colunas = append(colunas, fmt.Sprintf("%q", coluna))
		}
		operacoes = append(operacoes, fmt.Sprintf("%s(%s, %q, %s)",
			metodo, refTabela, nomeDoIndice, strings.Join(colunas, ", ")))
	}
	corpo := strings.Join(operacoes, ",\n\t\t")

	timestamp := quando.Format("2006_01_02_150405")
	arquivo := fmt.Sprintf("%s_create_%s.go", timestamp, fisica)

	caminho := filepath.Join(pastaDeMigrations(root, saida), "create_table", arquivo)

	declaracao := dynamicDeclarationName("Migration", arquivo)
	// O core só entra quando o arquivo realmente o usa: import sem uso não compila.
	// Quem o usa é o CreateIndex, então a condição é a mesma que gera o índice.
	blocoDeImports := "import migrate \"github.com/PhelipeViana/gokit/migration\"\n\n"
	if len(operacoes) > 1 {
		blocoDeImports = fmt.Sprintf("import (\n\tcore %q\n\tmigrate \"github.com/PhelipeViana/gokit/migration\"\n)\n\n", importCore)
	}

	conteudo := "package migrations\n\n" +
		blocoDeImports +
		i18n.Tf("imp_gen_doc", fisica) +
		"func " + declaracao + "() migrate.Definition {\n" +
		"\treturn migrate.Define(\n\t\t" + corpo + ",\n\t)\n}\n"

	// gofmt no arquivo gerado: a montagem por concatenação não acerta a
	// indentação do literal, e migration gerada suja apareceria no `gofmt -l` do
	// projeto de quem importou.
	formatado, err := format.Source([]byte(conteudo))
	if err != nil {
		return arquivoPlanejado{}, i18n.Errf("imp_format_failed", arquivo, err)
	}

	return arquivoPlanejado{Nome: arquivo, Caminho: caminho, Tabela: fisica, Conteudo: string(formatado)}, nil
}

// pastaDeMigrations resolve a pasta configurada, do mesmo jeito que o
// CreateScaffoldMigration: o import não pode inventar caminho, porque projeto
// legado aponta o output para outro lugar.
func pastaDeMigrations(root, saida string) string {
	caminho := filepath.FromSlash(saida)
	if filepath.IsAbs(caminho) {
		return caminho
	}
	return filepath.Join(root, caminho)
}

// registrarViewsMapeadas grava a definição de cada view do banco no layout MAPEADO,
// sem gerar migration nenhuma.
//
// View saiu do corpus: `migrate run` não cria view. O que existe é o REGISTRO —
// `<pasta de migrate>/views/<nome>/<dialeto>.sql` — e o BANCO é a verdade. Quem cria a
// view é a pessoa, no banco; o gokit lê e anota. Isso isola a responsabilidade: o
// migrate cuida do que o gokit declara e aplica, o mapper cuida do que ele só observa.
//
// O texto vai para o arquivo do DIALETO DE ORIGEM — `sqlserver.sql`, não
// `common.sql` — e essa é a decisão central: aquele SQL comprovadamente funciona num
// banco e é NÃO VERIFICADO nos outros três. Gravar como `common.sql` afirmaria uma
// portabilidade que ninguém checou, e o `migrate run` nos outros bancos aplicaria
// SQL que não é do dialeto deles. Com o arquivo por dialeto, os outros três falham
// com erro claro dizendo que falta a definição — que é a verdade.
//
// Converter automaticamente foi considerado e recusado: `TOP` do SQL Server,
// `ROWNUM` do Oracle e `LIMIT` dos outros dois não têm tradução confiável, e
// conversão errada viraria mentira gravada em histórico versionado.
func registrarViewsMapeadas(root, saida, dialect string, views map[string]ViewDoBanco, monitor *Monitor) ([]arquivoPlanejado, error) {
	nomes := make([]string, 0, len(views))
	for nome := range views {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)

	planejados := make([]arquivoPlanejado, 0, len(nomes))
	for _, nome := range nomes {
		view := views[nome]
		if strings.TrimSpace(view.SQL) == "" {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Objeto:  nome,
				Origem:  "view sem definição legível no catálogo",
				Decisao: "não declarada",
			})
			continue
		}

		// Sem arquivo .go e sem timestamp: a pasta da view é PLANA, uma por nome, com um
		// .sql por dialeto. O timestamp existia para casar com a migration, e não há
		// mais migration para casar.
		planejados = append(planejados, arquivoPlanejado{
			Nome:          nome,
			Tabela:        nome,
			SQLDaView:     view.SQL,
			PastaDaView:   filepath.Join(pastaDeMigrations(root, saida), "views", nome),
			DialetoDaView: dialect,
		})

		monitor.Registrar(Ocorrencia{
			Tipo:    OcorrenciaSQLNaoPortavel,
			Objeto:  nome,
			Origem:  "definição lida de " + dialect,
			Decisao: "gravada em " + dialect + ".sql; os outros três dialetos não têm definição e vão falhar até alguém escrevê-la",
		})
	}
	return planejados, nil
}

// importCoreDaView resolve o import do core para o arquivo de view.
func importCoreDaView(root string) string {
	if pRoot := projectRoot(root); pRoot != "" {
		if modulo, err := GetModuleName(pRoot); err == nil {
			return modulo + "/internal/gokit/core"
		}
	}
	return ""
}

// viewsForaDoCorpus devolve as views do banco que o corpus ainda não declara.
func viewsForaDoCorpus(root string, state config.ConfigState) (map[string]ViewDoBanco, error) {
	if state.Config == nil {
		return nil, i18n.Errf("scan_no_config")
	}
	connection := state.Config.Connections[state.ActiveClient]
	dialect := strings.ToLower(connection.Dialect)
	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return nil, i18n.Errf("run_dialect_unsupported", connection.Dialect)
	}

	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	doBanco, err := LerViewsDoBanco(ctx, db, dialect, connection.Schema)
	if err != nil {
		return nil, err
	}

	// O catálogo guarda o nome de catálogo; a comparação é pelo nome físico, em
	// minúsculas, como em todo o resto.
	pRoot := projectRoot(root)
	if pRoot == "" {
		pRoot = root
	}
	declaradas := map[string]bool{}
	for _, nome := range migrationgo.ViewNames(pRoot) {
		if fisico, tem := migrationgo.ViewPhysicalName(pRoot, nome); tem {
			declaradas[strings.ToLower(fisico)] = true
		}
	}

	pendentes := map[string]ViewDoBanco{}
	for nome, view := range doBanco {
		if !declaradas[nome] {
			pendentes[nome] = view
		}
	}
	return pendentes, nil
}

// registrarViewsNoCatalogo acrescenta as views importadas ao catálogo do core.
//
// Tem de acontecer antes de qualquer leitura do corpus: a migration gerada
// referencia core.View.X, e o parser recusa referência que não está no catálogo.
func registrarViewsNoCatalogo(pRoot string, novas map[string]ViewDoBanco) error {
	if len(novas) == 0 {
		return nil
	}
	todas := map[string]bool{}
	for _, nome := range migrationgo.ViewNames(pRoot) {
		if fisico, tem := migrationgo.ViewPhysicalName(pRoot, nome); tem {
			todas[fisico] = true
		}
	}
	for nome := range novas {
		todas[nome] = true
	}
	return migrationgo.WriteCoreViewCatalog(pRoot, todas)
}

// aliasesUnicos escolhe um apelido por tabela, garantindo que dois nomes físicos
// nunca gerem o mesmo identificador de catálogo.
//
// O normalizador come separador e caixa, então nomes distintos podem colidir:
// medido num schema real de 754 tabelas, `eventos_bkp435037` e `eventos_bkp_435037`
// geram os dois `EventosBkp435037`. Uma colisão basta para o `migrate validate`
// abortar o corpus inteiro — 754 tabelas travadas por um par.
//
// O apelido é o único lugar seguro para desempatar: ele é um nome livre, e o nome
// FÍSICO fica intocado. Renomear o físico mudaria a tabela que o DDL procura.
//
// A ordem é por nome físico, e não a de dependência, para o mesmo schema produzir
// sempre os mesmos apelidos, independente do grafo de FK.
func aliasesUnicos(tabelas []TabelaDoBanco, monitor *Monitor) map[string]string {
	fisicas := make([]string, 0, len(tabelas))
	for _, tabela := range tabelas {
		fisicas = append(fisicas, strings.ToLower(tabela.Nome))
	}
	sort.Strings(fisicas)

	escolhidos := make(map[string]string, len(fisicas))
	usados := make(map[string]string, len(fisicas))
	for _, fisica := range fisicas {
		alias := fisica
		identificador := migrationgo.ExportedIdentifier(alias)
		for tentativa := 2; ; tentativa++ {
			anterior, ocupado := usados[identificador]
			if !ocupado {
				break
			}
			// Sufixo em palavra, não em número solto: `_alias2` vira `Alias2` no
			// identificador, que não se confunde com dígito do nome original.
			alias = fmt.Sprintf("%s_alias%d", fisica, tentativa)
			identificador = migrationgo.ExportedIdentifier(alias)
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaAliasRenomeado,
				Tabela:  fisica,
				Origem:  fmt.Sprintf("apelido %q colidiria com %q no identificador %s", fisica, anterior, migrationgo.ExportedIdentifier(fisica)),
				Decisao: fmt.Sprintf("apelido virou %q; o nome físico não mudou", alias),
			})
		}
		usados[identificador] = fisica
		escolhidos[fisica] = alias
	}
	return escolhidos
}

// viewsImportaveis separa as views que podem ser declaradas das que não podem.
//
// Duas coisas impedem: nome que o corpus não sabe expressar, e colisão de
// identificador de catálogo. A colisão é a mais grave, e foi medida num schema real:
// `vw_competencias_contrib` e `vwcompetencias_contrib` geram o MESMO
// `VwCompetenciasContrib`, porque ViewIdentifier força o prefixo `vw_`. Com uma
// entrada só no catálogo, a migration da segunda view resolveria para a primeira — e
// criaria uma view com a definição da outra, em silêncio. Pular é a única saída
// honesta: renomear a view no banco é decisão de quem é dono dele.
func viewsImportaveis(views map[string]ViewDoBanco, monitor *Monitor) map[string]ViewDoBanco {
	nomes := make([]string, 0, len(views))
	for nome := range views {
		nomes = append(nomes, nome)
	}
	sort.Strings(nomes)

	aceitas := make(map[string]ViewDoBanco, len(nomes))
	porIdentificador := make(map[string]string, len(nomes))
	for _, nome := range nomes {
		if !acao.NomeFisicoValido(nome) {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Objeto:  nome,
				Origem:  "nome de view fora da convenção do corpus",
				Decisao: "não declarada: renomeie a view no banco e importe de novo",
			})
			continue
		}
		identificador := migrationgo.ViewIdentifier(nome)
		if anterior, ocupado := porIdentificador[identificador]; ocupado {
			monitor.Registrar(Ocorrencia{
				Tipo:    OcorrenciaNaoLido,
				Objeto:  nome,
				Origem:  fmt.Sprintf("gera o mesmo core.View.%s que a view %q", identificador, anterior),
				Decisao: "não declarada: declarar as duas faria uma apontar para a definição da outra",
			})
			continue
		}
		porIdentificador[identificador] = nome
		aceitas[nome] = views[nome]
	}
	return aceitas
}

// nomeGeradoDeIndiceUnico monta um nome válido para índice único cujo nome de origem
// o corpus não sabe expressar.
//
// Deriva de tabela + colunas, então é determinístico: reimportar o mesmo banco dá o
// mesmo nome, e o corpus não muda sem motivo. O prefixo `uq_` diz o que é.
func nomeGeradoDeIndiceUnico(tabela string, colunas []string) string {
	partes := make([]string, 0, len(colunas)+2)
	partes = append(partes, "uq", strings.ToLower(tabela))
	for _, coluna := range colunas {
		partes = append(partes, strings.ToLower(coluna))
	}
	// O limite de 30 é o do Oracle, o menor dos quatro; shortenIdentifier anexa hash
	// determinístico quando corta, então dois nomes longos não colapsam em um.
	//
	// O saneamento vem DEPOIS do corte, e é obrigatório: cortar no meio de um nome
	// pode deixar `_` no fim, e com o hash anexado sai `usr__9b0424` — underscore
	// duplo, que o validador de nome físico recusa. O corpus não aceitaria o arquivo
	// que ele mesmo gerou.
	return sanearNomeGerado(shortenIdentifier(strings.Join(partes, "_"), 30))
}

// sanearNomeGerado colapsa `_` repetido e apara as pontas.
//
// O underscore duplo é rejeitado de propósito pelo validador — `pessoas__ativas` e
// `pessoas_ativas` colidiriam —, então nome gerado não pode produzi-lo.
func sanearNomeGerado(nome string) string {
	var saida strings.Builder
	anterior := byte(0)
	for i := 0; i < len(nome); i++ {
		if nome[i] == '_' && anterior == '_' {
			continue
		}
		saida.WriteByte(nome[i])
		anterior = nome[i]
	}
	return strings.Trim(saida.String(), "_")
}
