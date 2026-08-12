package orm

import (
	"testing"
	"time"
)

// linhaUsers imita a struct que o fields.gen.go gera: coluna anulável é ponteiro,
// e o nome do campo Go é o Field.Name.
type linhaUsers struct {
	Id         int64
	Nome       string
	Email      string
	CidadeId   *int64
	Nascimento *time.Time
	Ativo      *bool
	Saldo      *float64
}

func ponteiroInt(v int64) *int64       { return &v }
func ponteiroBool(v bool) *bool        { return &v }
func ponteiroFloat(v float64) *float64 { return &v }
func ponteiroData(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

func exemplo() linhaUsers {
	return linhaUsers{
		Id: 7, Nome: "Ana Maria", Email: "ana@exemplo.com",
		CidadeId: ponteiroInt(3), Nascimento: ponteiroData("1990-05-17"),
		Ativo: ponteiroBool(true), Saldo: ponteiroFloat(1234.56),
	}
}

func TestMatchesDosOperadoresDeTexto(t *testing.T) {
	users := entidadeUsers()
	nome := StringColumn{campo(users, "nome")}
	linha := exemplo()

	casos := []struct {
		nome     string
		condicao Condition
		esperado bool
	}{
		{"Contains acha", nome.Contains("na Ma"), true},
		// Sensível à caixa: é o que o LIKE do SQL faz. MySQL e SQL Server acham de
		// todo jeito por collation do banco, o que é divergência de infraestrutura e
		// não da cláusula — está registrada como decisão aberta.
		{"Contains é sensível à caixa", nome.Contains("ANA"), false},
		{"Contains não acha", nome.Contains("joão"), false},
		{"StartsWith acha", nome.StartsWith("Ana"), true},
		{"StartsWith não acha", nome.StartsWith("Maria"), false},
		{"EndsWith acha", nome.EndsWith("Maria"), true},
		{"Equal exato", nome.Equal("Ana Maria"), true},
		{"Equal diferente", nome.Equal("Ana"), false},
		{"NotEqual", nome.NotEqual("Ana"), true},
		{"In com acerto", nome.In("Bia", "Ana Maria"), true},
		{"In sem acerto", nome.In("Bia", "Carla"), false},
		{"IsNull em coluna preenchida", nome.IsNull(), false},
		{"IsNotNull em coluna preenchida", nome.IsNotNull(), true},
	}
	for _, caso := range casos {
		if obtido := caso.condicao.Matches(linha); obtido != caso.esperado {
			t.Errorf("%s: esperado %v, obtido %v", caso.nome, caso.esperado, obtido)
		}
	}
}

// Número da linha vem como int64 e o literal do critério como int: sem conversão,
// a comparação falharia sempre.
func TestMatchesComparaNumerosDeTiposDiferentes(t *testing.T) {
	users := entidadeUsers()
	id := NumberColumn{campo(users, "id")}
	saldo := NumberColumn{campo(users, "saldo")}
	linha := exemplo()

	casos := map[string]struct {
		condicao Condition
		esperado bool
	}{
		"id igual":            {id.Equal(7), true},
		"id maior":            {id.GreaterThan(5), true},
		"id menor":            {id.LessThan(5), false},
		"id entre":            {id.Between(1, 10), true},
		"id fora do entre":    {id.Between(10, 20), false},
		"saldo decimal maior": {saldo.GreaterThan(1000), true},
		"saldo decimal menor": {saldo.LessOrEqual(1234.56), true},
		"id In":               {id.In(1, 7, 9), true},
	}
	for nome, caso := range casos {
		if obtido := caso.condicao.Matches(linha); obtido != caso.esperado {
			t.Errorf("%s: esperado %v, obtido %v", nome, caso.esperado, obtido)
		}
	}
}

func TestMatchesComData(t *testing.T) {
	users := entidadeUsers()
	nascimento := DateColumn{campo(users, "nascimento")}
	linha := exemplo()

	if !nascimento.After("1980-01-01").Matches(linha) {
		t.Error("After deveria ser verdadeiro")
	}
	if nascimento.Before("1980-01-01").Matches(linha) {
		t.Error("Before deveria ser falso")
	}
	if !nascimento.Between("1990-01-01", "1990-12-31").Matches(linha) {
		t.Error("Between deveria conter a data")
	}
	if !nascimento.Equal("1990-05-17").Matches(linha) {
		t.Error("Equal com texto de data deveria bater")
	}
}

// Coluna anulável vazia: o SQL não satisfaz comparação com NULL, nem mesmo o
// diferente. A avaliação em memória tem de seguir a mesma regra, senão o filtro e
// o if divergem — que é exatamente o que este recurso existe para evitar.
func TestMatchesTrataNuloComoOSQLTrata(t *testing.T) {
	users := entidadeUsers()
	cidade := NumberColumn{campo(users, "cidade_id")}
	linha := exemplo()
	linha.CidadeId = nil

	if cidade.Equal(3).Matches(linha) {
		t.Error("NULL = 3 deveria ser falso")
	}
	if cidade.NotEqual(3).Matches(linha) {
		t.Error("NULL <> 3 deveria ser falso, como no SQL")
	}
	if cidade.GreaterThan(0).Matches(linha) {
		t.Error("NULL > 0 deveria ser falso")
	}
	if !cidade.IsNull().Matches(linha) {
		t.Error("IsNull deveria ser verdadeiro")
	}
	if cidade.IsNotNull().Matches(linha) {
		t.Error("IsNotNull deveria ser falso")
	}
}

// A árvore booleana em memória tem de dar a mesma resposta que o parêntese do SQL.
func TestMatchesRespeitaAndOr(t *testing.T) {
	users := entidadeUsers()
	nome := StringColumn{campo(users, "nome")}
	id := NumberColumn{campo(users, "id")}
	linha := exemplo()

	if !And(nome.Contains("Ana"), id.Equal(7)).Matches(linha) {
		t.Error("AND de duas verdadeiras deveria ser verdadeiro")
	}
	if And(nome.Contains("Ana"), id.Equal(99)).Matches(linha) {
		t.Error("AND com uma falsa deveria ser falso")
	}
	if !Or(nome.Contains("joão"), id.Equal(7)).Matches(linha) {
		t.Error("OR com uma verdadeira deveria ser verdadeiro")
	}
	if Or(nome.Contains("joão"), id.Equal(99)).Matches(linha) {
		t.Error("OR de duas falsas deveria ser falso")
	}
	// Aninhado: a AND (b OR c)
	composto := And(id.Equal(7), Or(nome.Contains("joão"), nome.Contains("Maria")))
	if !composto.Matches(linha) {
		t.Error("a AND (b OR c) deveria ser verdadeiro")
	}
}

// Filtro dinâmico inativo não filtra no SQL; em memória, não pode excluir a linha.
func TestMatchesIgnoraCondicaoInativa(t *testing.T) {
	users := entidadeUsers()
	inativa := Filter(campo(users, "nome"), Equal, "")
	if !Matches(inativa, exemplo()) {
		t.Error("condição inativa não deveria excluir a linha")
	}
}

// O EXISTS de relação precisa do banco: Evaluable avisa antes, e Matches não
// inventa resposta.
func TestExistsDeRelacaoNaoEAvaliavelEmMemoria(t *testing.T) {
	users := entidadeUsers()
	nome := StringColumn{campo(users, "nome")}

	if !Evaluable(nome.Contains("Ana")) {
		t.Error("condição simples deveria ser avaliável")
	}
	if !Evaluable(And(nome.Contains("Ana"), nome.IsNotNull())) {
		t.Error("grupo de condições simples deveria ser avaliável")
	}

	relacao := Relation{}
	if Evaluable(relacao.Exists()) {
		t.Error("EXISTS de relação não deveria ser avaliável em memória")
	}
	if Evaluable(And(nome.Contains("Ana"), relacao.Exists())) {
		t.Error("grupo contendo EXISTS não deveria ser avaliável")
	}
}

// Também funciona sobre o Record do resultado agrupado, e sobre ponteiro da linha.
func TestMatchesAceitaRecordEPonteiro(t *testing.T) {
	users := entidadeUsers()
	nome := StringColumn{campo(users, "nome")}
	linha := exemplo()

	if !nome.Contains("Ana").Matches(&linha) {
		t.Error("ponteiro para a linha deveria funcionar")
	}
	registro := Record{"nome": Value{bruto: "Ana Maria"}}
	if !nome.Contains("Ana").Matches(registro) {
		t.Error("Record deveria funcionar")
	}
}

// Coluna que não veio na projeção não pode virar panic.
func TestMatchesComColunaAusenteNaoEntraEmPanico(t *testing.T) {
	users := entidadeUsers()
	saldo := NumberColumn{campo(users, "saldo")}
	type parcial struct{ Nome string }

	if saldo.GreaterThan(0).Matches(parcial{Nome: "Ana"}) {
		t.Error("coluna ausente deveria devolver falso")
	}
}
