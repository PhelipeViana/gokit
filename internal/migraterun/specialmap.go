package migraterun

// Special Mapper — o comando que mapeia view, function e procedure.
//
// O gokit não gerencia esses três: ele MAPEIA. O banco é a verdade. A pessoa cria o
// objeto no banco; o mapper lê, registra a definição por dialeto em arquivo e (a seguir)
// gera o acessador para o código. **O mapper nunca cria nada no banco** — é decisão de
// projeto, não limitação: traduzir SQL entre dialetos não é confiável, e o que se grava
// aqui é histórico.
//
// A ordem dos passos não é arbitrária:
//
//	1. ler o catálogo         → precisa vir antes de tudo
//	2. registrar <dialeto>.sql → o que existe no banco ativo
//	3. .sql comentado          → depende de saber o que existe em OUTRO dialeto
//	4. podar pasta órfã        → antes do acessador, senão gero para pasta que vai morrer
//	5. gerar acessador         → precisa do inventário completo dos passos 1-4
//
// Este arquivo cobre 1 a 4. O passo 5 é o gerador de acessadores.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/aviso"
	"github.com/PhelipeViana/gokit/internal/cliui"
	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
)

// pastasPorTipo é o nome da pasta de cada tipo. Plural, para ler como coleção.
var pastasPorTipo = map[string]string{
	EspecialView:      "views",
	EspecialFunction:  "functions",
	EspecialProcedure: "procedures",
}

// tiposEmOrdem fixa a ordem de apresentação. View primeiro porque é a que o código
// consome de imediato; procedure por último porque é a menos portável.
var tiposEmOrdem = []string{EspecialView, EspecialFunction, EspecialProcedure}

// resultadoDoMapper é o que aconteceu com um objeto.
type resultadoDoMapper struct {
	Tipo      string
	Nome      string
	Situacao  string // "registrado" | "pendente" | "podado"
	Dialeto   string
	Detalhe   string
	Caminho   string
	NumColuna int
	NumParam  int
}

// SpecialMap lê os objetos especiais do banco ativo e sincroniza o registro em disco.
//
// Sem `confirmar` apenas MOSTRA o que faria — mesma regra do import, pelo mesmo motivo:
// o registro é versionado, e escrever arquivo não se desfaz sozinho.
func SpecialMap(root string, state config.ConfigState, confirmar bool) error {
	if state.Config == nil {
		return cliui.NewUserError(i18n.T("scan_no_config"), i18n.T("spc_no_config_fix"))
	}
	connection := state.Config.Connections[state.ActiveClient]
	dialect := strings.ToLower(connection.Dialect)
	if dialect == "" {
		return cliui.NewUserError(i18n.T("spc_no_dialect"), i18n.T("spc_no_config_fix"))
	}

	driver := map[string]string{"oracle": "oracle", "postgres": "pgx", "mysql": "mysql", "sqlserver": "sqlserver"}[dialect]
	if driver == "" {
		return i18n.Errf("run_dialect_unsupported", connection.Dialect)
	}
	db, err := sql.Open(driver, connection.BuildURL())
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancelar := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancelar()

	// PASSO 1 — ler o catálogo.
	objetos, problemas := LerObjetosEspeciais(ctx, db, dialect, connection.Schema)
	for _, problema := range problemas {
		fmt.Println(cliui.Warning(i18n.Tf("spc_read_partial", problema)))
	}

	base := pastaDeEspeciais(root, state)
	monitor := NovoMonitor("special map", state.ActiveClient, dialect)

	total := 0
	for _, tipo := range tiposEmOrdem {
		total += len(objetos[tipo])
	}
	fmt.Printf("\n%s\n", i18n.Tf("spc_header", state.ActiveClient, dialect, total))

	var acoes []resultadoDoMapper

	// PASSO 2 — registrar o que existe no banco ativo.
	for _, tipo := range tiposEmOrdem {
		for _, nome := range NomesOrdenadosDeEspeciais(objetos[tipo]) {
			objeto := objetos[tipo][nome]
			pasta := filepath.Join(base, pastasPorTipo[tipo], nome)
			if strings.TrimSpace(objeto.SQL) == "" {
				monitor.Registrar(Ocorrencia{
					Tipo:    OcorrenciaNaoLido,
					Objeto:  tipo + " " + nome,
					Origem:  "definição vazia no catálogo",
					Decisao: "não registrada: sem texto não há o que gravar",
				})
				continue
			}
			acoes = append(acoes, resultadoDoMapper{
				Tipo: tipo, Nome: nome, Situacao: "registrado", Dialeto: dialect,
				Caminho:   filepath.Join(pasta, dialect+".sql"),
				NumColuna: len(objeto.Colunas), NumParam: len(objeto.Parametros),
			})
		}
	}

	// PASSO 3 — pasta que existe em disco mas o objeto não está no dialeto ATIVO.
	//
	// Isso é o caso da migração: a view veio do SQL Server e o Oracle ainda não a tem.
	// O arquivo comentado é o pedido de análise — e é a única coisa que o mapper
	// escreve sem ter lido do banco, justamente porque não é definição, é lembrete.
	emDisco, err := pastasRegistradas(base)
	if err != nil {
		return err
	}
	for _, registro := range emDisco {
		if _, existeNoBanco := objetos[registro.Tipo][registro.Nome]; existeNoBanco {
			continue
		}
		arquivoDoDialeto := filepath.Join(registro.Pasta, dialect+".sql")
		if temConteudo(arquivoDoDialeto) {
			continue
		}
		// PASSO 4 — podar: nada no banco e nenhum arquivo com conteúdo.
		if !registro.TemAlgumConteudo {
			acoes = append(acoes, resultadoDoMapper{
				Tipo: registro.Tipo, Nome: registro.Nome, Situacao: "podado",
				Caminho: registro.Pasta,
				Detalhe: i18n.T("spc_prune_reason"),
			})
			continue
		}
		acoes = append(acoes, resultadoDoMapper{
			Tipo: registro.Tipo, Nome: registro.Nome, Situacao: "pendente", Dialeto: dialect,
			Caminho: arquivoDoDialeto,
			Detalhe: strings.Join(registro.DialetosComConteudo, ", "),
		})
		monitor.Registrar(Ocorrencia{
			Tipo:    OcorrenciaSQLNaoPortavel,
			Objeto:  registro.Tipo + " " + registro.Nome,
			Origem:  "definição existe em " + strings.Join(registro.DialetosComConteudo, ", "),
			Decisao: "falta a de " + dialect + "; crie o objeto no banco e rode o mapper de novo",
		})
	}

	imprimirAcoesDoMapper(acoes)

	if !confirmar {
		fmt.Printf("\n%s\n\n", i18n.T("spc_needs_confirm"))
		return nil
	}

	for _, acao := range acoes {
		switch acao.Situacao {
		case "registrado":
			objeto := objetos[acao.Tipo][acao.Nome]
			if err := os.MkdirAll(filepath.Dir(acao.Caminho), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(acao.Caminho, []byte(objeto.SQL+"\n"), 0o644); err != nil {
				return err
			}
		case "pendente":
			if err := os.MkdirAll(filepath.Dir(acao.Caminho), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(acao.Caminho, []byte(pedidoDeAnalise(acao, dialect)), 0o644); err != nil {
				return err
			}
		case "podado":
			if err := os.RemoveAll(acao.Caminho); err != nil {
				return err
			}
		}
	}

	if caminho, err := monitor.Escrever(root, state.Config.Output.Docs); err != nil {
		fmt.Println(cliui.Warning(i18n.Tf("mon_falhou", err)))
	} else if caminho != "" {
		fmt.Printf("\n%s\n", i18n.Tf("mon_gravado", monitor.Total, caminho))
		for _, linha := range monitor.Resumo() {
			fmt.Printf("  %s %s\n", cliui.Muted("·"), linha)
		}
		monitor.Notificar(aviso.DoEstado(state), caminho)
	}

	fmt.Printf("\n  %s %s\n\n", cliui.Success("✓ OK"), i18n.Tf("spc_done", len(acoes)))
	return nil
}

// pedidoDeAnalise monta o conteúdo do .sql vazio.
//
// É comentário SQL puro: o arquivo tem de continuar sendo um .sql válido, para o editor
// colorir e para ninguém achar que é lixo. E diz de onde copiar, porque é isso que a
// pessoa vai querer saber ao abrir.
func pedidoDeAnalise(acao resultadoDoMapper, dialect string) string {
	var texto strings.Builder
	texto.WriteString("-- " + i18n.Tf("spc_placeholder_title", acao.Tipo, acao.Nome, dialect) + "\n--\n")
	for _, linha := range strings.Split(i18n.Tf("spc_placeholder_body", acao.Detalhe, dialect), "\n") {
		texto.WriteString("-- " + linha + "\n")
	}
	return texto.String()
}

// registroEmDisco é uma pasta de objeto especial já existente.
type registroEmDisco struct {
	Tipo                string
	Nome                string
	Pasta               string
	TemAlgumConteudo    bool
	DialetosComConteudo []string
}

// pastasRegistradas varre o disco. É o que permite ao mapper falar do que NÃO está no
// banco ativo — sem isso ele só saberia acrescentar, nunca apontar o que falta.
func pastasRegistradas(base string) ([]registroEmDisco, error) {
	var registros []registroEmDisco
	for _, tipo := range tiposEmOrdem {
		raiz := filepath.Join(base, pastasPorTipo[tipo])
		entradas, err := os.ReadDir(raiz)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entrada := range entradas {
			if !entrada.IsDir() {
				continue
			}
			pasta := filepath.Join(raiz, entrada.Name())
			arquivos, err := os.ReadDir(pasta)
			if err != nil {
				return nil, err
			}
			registro := registroEmDisco{Tipo: tipo, Nome: entrada.Name(), Pasta: pasta}
			for _, arquivo := range arquivos {
				if arquivo.IsDir() || !strings.HasSuffix(arquivo.Name(), ".sql") {
					continue
				}
				if temConteudo(filepath.Join(pasta, arquivo.Name())) {
					registro.TemAlgumConteudo = true
					registro.DialetosComConteudo = append(registro.DialetosComConteudo,
						strings.TrimSuffix(arquivo.Name(), ".sql"))
				}
			}
			sort.Strings(registro.DialetosComConteudo)
			registros = append(registros, registro)
		}
	}
	return registros, nil
}

// temConteudo diz se o .sql tem SQL de verdade — não só comentário e espaço.
//
// O arquivo de pedido de análise é todo comentário, e tratá-lo como conteúdo faria o
// mapper achar que a definição já existe. É essa distinção que deixa a regra de poda
// funcionar: "arquivos vazios" inclui o que só tem lembrete.
func temConteudo(caminho string) bool {
	dados, err := os.ReadFile(caminho)
	if err != nil {
		return false
	}
	for _, linha := range strings.Split(string(dados), "\n") {
		limpa := strings.TrimSpace(linha)
		if limpa == "" || strings.HasPrefix(limpa, "--") {
			continue
		}
		return true
	}
	return false
}

// pastaDeEspeciais devolve a raiz do registro dos objetos especiais.
//
// Fica FORA da pasta de migrations: view, function e procedure não são migration, e
// deixá-las lá era o que confundia a responsabilidade.
func pastaDeEspeciais(root string, state config.ConfigState) string {
	caminho := "internal/gokit/special"
	if state.Config != nil && strings.TrimSpace(state.Config.Output.Special) != "" {
		caminho = strings.TrimSpace(state.Config.Output.Special)
	}
	if filepath.IsAbs(caminho) {
		return caminho
	}
	return filepath.Join(root, filepath.FromSlash(caminho))
}

// imprimirAcoesDoMapper mostra o que será feito, agrupado por tipo.
func imprimirAcoesDoMapper(acoes []resultadoDoMapper) {
	for _, tipo := range tiposEmOrdem {
		var doTipo []resultadoDoMapper
		for _, acao := range acoes {
			if acao.Tipo == tipo {
				doTipo = append(doTipo, acao)
			}
		}
		if len(doTipo) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", i18n.Tf("spc_group", pastasPorTipo[tipo], len(doTipo)))
		for _, acao := range doTipo {
			switch acao.Situacao {
			case "registrado":
				detalhe := ""
				if acao.NumColuna > 0 {
					detalhe = i18n.Tf("spc_detail_columns", acao.NumColuna)
				} else if acao.NumParam > 0 {
					detalhe = i18n.Tf("spc_detail_params", acao.NumParam)
				}
				fmt.Printf("  %s %-44s %s\n", cliui.Success("+"), acao.Nome, cliui.Muted(detalhe))
			case "pendente":
				fmt.Printf("  %s %-44s %s\n", cliui.Warning("?"), acao.Nome,
					cliui.Muted(i18n.Tf("spc_detail_pending", acao.Detalhe)))
			case "podado":
				fmt.Printf("  %s %-44s %s\n", cliui.Muted("-"), acao.Nome, cliui.Muted(acao.Detalhe))
			}
		}
	}
}
