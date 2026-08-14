// Package aviso classifica o que merece notificação e entrega em um ou mais canais.
//
// A razão de existir é uma distinção que o motor já fazia e não usava: `cliui.UserError`
// é o erro MAPEADO — ele carrega mensagem e solução, porque alguém já encontrou aquele
// caso, entendeu e escreveu o que fazer. Erro cru é o oposto: ou é caso de banco que
// ninguém previu, ou é defeito do motor. Enquanto esse não chega a quem mantém o gokit,
// cada usuário topa com ele sozinho e o motor não melhora.
//
// O componente é genérico por dois motivos práticos. Primeiro, a CLASSIFICAÇÃO é o que
// decide o envio, e ela é configurável: um projeto pode querer só erro não mapeado,
// outro pode querer os casos atípicos também. Segundo, o canal é uma interface — hoje
// só existe Slack, e trocar ou somar canal não deve mexer em quem notifica.
//
// Nada aqui pode derrubar o comando. Falha de notificação vira aviso na saída de erro:
// o usuário está no meio de uma migration, e webhook fora do ar não é problema dele.
package aviso

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Produto é o nome que aparece no cabeçalho, igual ao que já circula no canal.
const Produto = "AgendaGoKit"

// Nivel é a gravidade de um aviso, e é o único eixo do filtro.
//
// Quem classifica é o GOKIT: ele sabe o que aconteceu e quão sério é. Quem recebe
// apenas escolhe quais níveis quer. Essa separação importa porque a supressão não pode
// morar no ponto de emissão — se o gokit decidir não emitir, nenhum projeto consegue
// pedir aquele aviso depois.
type Nivel string

const (
	// NivelInfo: aconteceu, já está resolvido, ninguém precisa agir. O apelido que o
	// import desempatou sozinho é disso.
	NivelInfo Nivel = "info"
	// NivelWarning: ponto de preocupação. Nada falhou, mas algo do banco não é coberto
	// pelo motor e degradou — a lacuna afeta qualquer banco parecido.
	NivelWarning Nivel = "warning"
	// NivelError: erro de uso. É o cliui.UserError, que carrega mensagem E solução
	// porque alguém já encontrou o caso e escreveu o que fazer.
	NivelError Nivel = "error"
	// NivelDanger: o mais grave. O gokit falhou com erro cru, sem solução escrita:
	// ou é caso de banco que ninguém previu, ou é defeito do motor.
	NivelDanger Nivel = "danger"
)

// niveisPorPadrao é o que se envia quando o projeto não configura filtro.
//
// Tudo menos info. Um canal de mantenedor provavelmente quer só danger; alguém
// acompanhando o próprio trabalho quer error também. O padrão pega o meio: não cala o
// que pede ação, e não enche o canal com o que já está resolvido.
var niveisPorPadrao = map[Nivel]bool{
	NivelWarning: true,
	NivelError:   true,
	NivelDanger:  true,
}

// Aviso é o que se notifica.
type Aviso struct {
	Nivel Nivel
	// Acao aparece em caixa alta no cabeçalho, como nas notificações que o canal já
	// recebe: "DROP BANCO", "ERRO NÃO MAPEADO".
	Acao  string
	Emoji string
	// Corpo é a prosa: o que aconteceu e o que a pessoa precisa fazer.
	Corpo string
	// Contexto entra no relatório colável — comando, arquivo, tabela, o que ajudar
	// quem for resolver a agir sem pedir mais informação.
	Contexto map[string]string
	// Bruto é o texto que o banco ou o Go devolveu, sem enfeite. É o que permite
	// reproduzir o caso, e por isso vai em bloco de código.
	Bruto string
}

// Canal é para onde um aviso vai.
type Canal interface {
	Enviar(texto string) error
	Nome() string
}

// Ambiente descreve de onde o aviso saiu, e é igual para todos os avisos da execução.
type Ambiente struct {
	Projeto string
	Cliente string
	Dialeto string
	Versao  string
}

// Central decide o que enviar e entrega nos canais.
type Central struct {
	canais   []Canal
	permite  map[Nivel]bool
	ambiente Ambiente
}

// Nova monta a central. `niveis` vem da configuração do projeto; vazio usa o padrão.
func Nova(ambiente Ambiente, niveis []string, canais ...Canal) Central {
	permite := niveisPorPadrao
	if len(niveis) > 0 {
		permite = map[Nivel]bool{}
		for _, nivel := range niveis {
			permite[Nivel(strings.TrimSpace(strings.ToLower(nivel)))] = true
		}
	}
	// Canal nulo é descartado aqui para que quem chama não precise checar.
	uteis := make([]Canal, 0, len(canais))
	for _, canal := range canais {
		if canal != nil {
			uteis = append(uteis, canal)
		}
	}
	return Central{canais: uteis, permite: permite, ambiente: ambiente}
}

// Ativa diz se há canal e filtro que permitam alguma coisa.
func (c Central) Ativa() bool { return len(c.canais) > 0 }

// Notificar entrega o aviso nos canais, se a classificação permitir.
func (c Central) Notificar(aviso Aviso) {
	if !c.Ativa() || !c.permite[aviso.Nivel] {
		return
	}
	texto := c.Formatar(aviso)
	for _, canal := range c.canais {
		if err := canal.Enviar(texto); err != nil {
			fmt.Fprintf(os.Stderr, "aviso: não foi possível notificar %s: %v\n", canal.Nome(), err)
		}
	}
}

// Formatar monta a mensagem.
//
// O formato segue a convenção que o canal já usa — cabeçalho com emoji e ação em
// caixa alta, depois quem, projeto e cliente. Inventar outro faria as notificações do
// mesmo produto parecerem de produtos diferentes.
//
// O fim é um RELATÓRIO COLÁVEL, e essa é a parte que decide a utilidade: o gokit não
// tem IA dentro, então quem recebe precisa poder colar o bloco em uma conversa e ter
// ali o suficiente para resolver, sem acesso à máquina de quem rodou.
func (c Central) Formatar(aviso Aviso) string {
	var texto strings.Builder

	emoji := aviso.Emoji
	if emoji == "" {
		emoji = ":warning:"
	}
	fmt.Fprintf(&texto, "%s %s · %s\n", emoji, Produto, strings.ToUpper(aviso.Acao))
	fmt.Fprintf(&texto, "Quem: %s\n", QuemEsta())
	fmt.Fprintf(&texto, "Projeto: %s\n", ouEntao(c.ambiente.Projeto, "(sem módulo)"))
	fmt.Fprintf(&texto, "Cliente: %s (%s)\n", ouEntao(c.ambiente.Cliente, "(sem cliente)"), ouEntao(c.ambiente.Dialeto, "?"))

	if aviso.Corpo != "" {
		fmt.Fprintf(&texto, "\n%s\n", aviso.Corpo)
	}

	fmt.Fprintf(&texto, "\n%s\n```\n%s```", legendaDoRelatorio, c.relatorio(aviso))
	return texto.String()
}

const legendaDoRelatorio = "Relatório para resolver — cole o bloco abaixo:"

// relatorio monta o bloco colável: contexto em linhas `chave: valor` e o texto cru
// no fim, que é o que costuma ter a resposta.
func (c Central) relatorio(aviso Aviso) string {
	var bloco strings.Builder

	fixos := [][2]string{
		{"nivel", string(aviso.Nivel)},
		{"produto", Produto + " " + ouEntao(c.ambiente.Versao, "development")},
		{"projeto", c.ambiente.Projeto},
		{"cliente", c.ambiente.Cliente},
		{"dialeto", c.ambiente.Dialeto},
	}
	for _, par := range fixos {
		if par[1] != "" {
			fmt.Fprintf(&bloco, "%s: %s\n", par[0], par[1])
		}
	}

	chaves := make([]string, 0, len(aviso.Contexto))
	for chave := range aviso.Contexto {
		chaves = append(chaves, chave)
	}
	sort.Strings(chaves)
	for _, chave := range chaves {
		if valor := strings.TrimSpace(aviso.Contexto[chave]); valor != "" {
			fmt.Fprintf(&bloco, "%s: %s\n", chave, valor)
		}
	}

	if bruto := strings.TrimSpace(aviso.Bruto); bruto != "" {
		fmt.Fprintf(&bloco, "\n%s\n", Recortar(bruto, 2000))
	}
	return bloco.String()
}

// QuemEsta identifica quem rodou, no formato `nome <email>` que o canal já usa.
//
// Vem do git, e não do usuário do sistema: é o nome que a equipe reconhece, e é o
// mesmo que assina o commit da migration que causou o problema.
func QuemEsta() string {
	nome := gitConfig("user.name")
	email := gitConfig("user.email")
	switch {
	case nome != "" && email != "":
		return fmt.Sprintf("%s <%s>", nome, email)
	case nome != "":
		return nome
	case email != "":
		return email
	}
	if maquina, err := os.Hostname(); err == nil {
		return maquina
	}
	return "(desconhecido)"
}

func gitConfig(chave string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	saida, err := exec.CommandContext(ctx, "git", "config", "--get", chave).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(saida))
}

func ouEntao(valor, padrao string) string {
	if strings.TrimSpace(valor) == "" {
		return padrao
	}
	return valor
}

// Recortar limita o texto sem perder o começo, que é onde está a causa.
func Recortar(texto string, limite int) string {
	texto = strings.TrimSpace(texto)
	if len(texto) <= limite {
		return texto
	}
	return texto[:limite] + "\n… (cortado)"
}
