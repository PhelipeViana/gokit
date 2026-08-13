package orm

import (
	"strings"
	"testing"
	"time"
)

// A forma tipada tem de produzir exatamente o mesmo SQL e os mesmos argumentos que
// a literal de mapa. Ela é uma porta de entrada nova para a mesma estrutura — se o
// SQL divergir, são duas escritas diferentes com cara de uma.
func TestSetProduzOMesmoSQLQueOMapa(t *testing.T) {
	users := entidadeUsers()
	nascimento := time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)

	tipado := Set(
		StringCol(campo(users, "nome")).Is("Ana"),
		StringCol(campo(users, "email")).Is("ana@exemplo.com"),
		NumberCol(campo(users, "saldo")).Is(10.5),
		BoolCol(campo(users, "ativo")).Is(true),
		DateCol(campo(users, "nascimento")).Is(nascimento),
	)
	mapa := Values{
		campo(users, "nome"):       "Ana",
		campo(users, "email"):      "ana@exemplo.com",
		campo(users, "saldo"):      10.5,
		campo(users, "ativo"):      true,
		campo(users, "nascimento"): nascimento,
	}

	for _, dialeto := range []Dialect{MySQL, Postgres, Oracle, SQLServer} {
		a, err := compileInsert(users, []Values{tipado}, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s tipado: %v", dialeto, err)
		}
		b, err := compileInsert(users, []Values{mapa}, CompileOptions{Dialect: dialeto})
		if err != nil {
			t.Fatalf("%s mapa: %v", dialeto, err)
		}
		if a.SQL != b.SQL {
			t.Errorf("%s: SQL divergiu\n  tipado %s\n  mapa   %s", dialeto, a.SQL, b.SQL)
		}
		if len(a.Args) != len(b.Args) {
			t.Fatalf("%s: %d args no tipado contra %d no mapa", dialeto, len(a.Args), len(b.Args))
		}
		for i := range a.Args {
			if a.Args[i] != b.Args[i] {
				t.Errorf("%s: argumento %d divergiu: %v contra %v", dialeto, i+1, a.Args[i], b.Args[i])
			}
		}
	}
}

// A data passa pelo DateValue, então texto em formato conhecido chega ao driver já
// como time.Time — igual ao lado do filtro.
func TestSetConverteDataComoOFiltro(t *testing.T) {
	users := entidadeUsers()
	valores := Set(DateCol(campo(users, "nascimento")).Is("1990-05-17"))

	_, dados, err := valores.ordenar(users)
	if err != nil {
		t.Fatal(err)
	}
	if len(dados) != 1 {
		t.Fatalf("esperado 1 valor, obtido %d", len(dados))
	}
	instante, ok := dados[0].(time.Time)
	if !ok {
		t.Fatalf("o texto deveria ter virado time.Time, veio %T", dados[0])
	}
	if instante.Year() != 1990 || instante.Month() != 5 || instante.Day() != 17 {
		t.Errorf("data convertida errada: %v", instante)
	}
}

// É o único erro que a forma tipada pega e a literal de mapa não: com chave não
// constante, o Go aceita a repetição e fica com a última, em silêncio. Num INSERT
// isso é o valor errado gravado sem aviso.
func TestSetRecusaColunaRepetida(t *testing.T) {
	users := entidadeUsers()
	nome := StringCol(campo(users, "nome"))

	valores := Set(nome.Is("primeiro"), nome.Is("segundo"))
	_, _, err := valores.ordenar(users)
	if err == nil {
		t.Fatal("coluna atribuída duas vezes deveria falhar")
	}
	if !strings.Contains(err.Error(), "duas vezes") || !strings.Contains(err.Error(), "nome") {
		t.Errorf("mensagem inesperada: %v", err)
	}
}

// O erro chega ao terminal, não fica preso no Values.
func TestErroDeAutoriaChegaAoTerminal(t *testing.T) {
	users := entidadeUsers()
	nome := StringCol(campo(users, "nome"))

	_, err := modelo(users).Insert(nil, Set(nome.Is("a"), nome.Is("b")))
	if err == nil || !strings.Contains(err.Error(), "duas vezes") {
		t.Fatalf("esperado o erro de coluna repetida no Insert, veio: %v", err)
	}
}

// SetNull em coluna obrigatória nunca é intenção legítima: quem quer o padrão da
// coluna omite a coluna. Falhar aqui troca o erro do driver por uma frase que diz
// o que fazer.
func TestSetNullRespeitaANulidadeDaColuna(t *testing.T) {
	users := entidadeUsers()

	// saldo é anulável na entidade de teste.
	valores := Set(NumberCol(campo(users, "saldo")).SetNull())
	campos, dados, err := valores.ordenar(users)
	if err != nil {
		t.Fatalf("SetNull em coluna anulável deveria passar: %v", err)
	}
	if len(campos) != 1 || dados[0] != nil {
		t.Errorf("esperado um NULL, obtido %v", dados)
	}

	// nome é obrigatória.
	_, _, err = Set(StringCol(campo(users, "nome")).SetNull()).ordenar(users)
	if err == nil {
		t.Fatal("SetNull em coluna obrigatória deveria falhar")
	}
	if !strings.Contains(err.Error(), "omita a coluna") {
		t.Errorf("a mensagem deveria dizer o que fazer: %v", err)
	}
}

// A forma tipada serve nas quatro escritas, porque Set devolve Values e nenhuma
// assinatura mudou.
func TestSetServeNasQuatroEscritas(t *testing.T) {
	users := entidadeUsers()
	m := modelo(users)
	valores := Set(
		StringCol(campo(users, "nome")).Is("Ana"),
		StringCol(campo(users, "email")).Is("ana@exemplo.com"),
	)

	// Sem conexão todas param em ErrNoConnection — o que se prova é que compilam
	// e que o Values tipado é aceito onde o mapa era.
	if _, err := m.Insert(nil, valores); err == nil {
		t.Error("Insert deveria falhar sem conexão")
	}
	if _, err := m.Where(condition(campo(users, "id"), Equal, 1)).Update(nil, valores); err == nil {
		t.Error("Update deveria falhar sem conexão")
	}
	if _, err := m.Upsert(nil, valores, StringCol(campo(users, "email"))); err == nil {
		t.Error("Upsert deveria falhar sem conexão")
	}
	if _, err := m.InsertIgnore(nil, valores, StringCol(campo(users, "email"))); err == nil {
		t.Error("InsertIgnore deveria falhar sem conexão")
	}
}

// Set vazio é o mesmo que Values vazio: o terminal recusa por falta de valores, e
// não por causa da forma usada.
func TestSetVazio(t *testing.T) {
	users := entidadeUsers()
	if _, _, err := Set().ordenar(users); err != nil {
		t.Errorf("Set vazio não deveria carregar erro próprio: %v", err)
	}
	if _, err := compileInsert(users, []Values{Set()}, CompileOptions{Dialect: MySQL}); err == nil {
		t.Error("Insert sem valores deveria falhar")
	}
}
