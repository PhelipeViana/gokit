package acao

import (
	"strings"
	"testing"
)

func TestValidarRejectsUnsafePhysicalTableNames(t *testing.T) {
	invalid := []string{"phelipe gabriel", "Pessoas", "pessoas-ativas", "pessoas__ativas", "1pessoas"}
	for _, name := range invalid {
		operation := Nova(CreateTable, name, NovaColuna("id").Integer()).Alias("pessoas")
		if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "snake_case") {
			t.Fatalf("nome %q deveria ser recusado, obtido: %v", name, err)
		}
	}
}

func TestValidarAcceptsSnakeCasePhysicalTableName(t *testing.T) {
	operation := Nova(CreateTable, "phelipe_gabriel", NovaColuna("id").Integer()).Alias("phelipeGabriel")
	if err := Validar(operation); err != nil {
		t.Fatalf("nome snake_case deveria ser aceito: %v", err)
	}
}

func TestValidarRejectsUnsafeColumnNames(t *testing.T) {
	tests := []Operacao{
		Nova(CreateTable, "pessoas", NovaColuna("coluna separada tambem vai").Integer()).Alias("pessoas"),
		Nova(AddColumn, "pessoas", NovaColuna("nome completo").Varchar(255)),
		Nova(AlterColumn, "pessoas", NovaColuna("Nome").Varchar(255)),
		Nova(DropColumn, "pessoas", NovaColuna("nome-completo")),
		Nova(AddForeignKey, "pessoas", NovaColuna("cidade id").References("cidades", "id")),
	}
	for _, operation := range tests {
		if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "nome de coluna") {
			t.Fatalf("operação %s deveria recusar coluna inválida, obtido: %v", operation.Kind, err)
		}
	}
}

func TestValidarAcceptsSnakeCaseColumnName(t *testing.T) {
	operation := Nova(AddColumn, "pessoas", NovaColuna("nome_completo").Varchar(255))
	if err := Validar(operation); err != nil {
		t.Fatalf("coluna snake_case deveria ser aceita: %v", err)
	}
}

func TestValidarRejectsUnsafeAliasName(t *testing.T) {
	operation := Nova(CreateTable, "phelipe_gabriel", NovaColuna("id").Integer()).Alias("phelipe gabriel")
	if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "lowerCamelCase") {
		t.Fatalf("alias inválido deveria ser recusado, obtido: %v", err)
	}
}

func TestValidarRejectsUnsafeRenameTableName(t *testing.T) {
	operation := Operacao{Kind: string(RenameTable), Table: "pessoas", NewName: "Phelipe Gabriel"}
	if err := Validar(operation); err == nil || !strings.Contains(err.Error(), "snake_case") {
		t.Fatalf("RenameTable deveria recusar nome inválido, obtido: %v", err)
	}
}


// A regra de nome foi relaxada para o que os QUATRO bancos aceitam, e não para o que
// é bonito. Medido: `autenticação` e `data_` criam, inserem e leem em MySQL,
// Postgres, Oracle e SQL Server, porque o gokit cita todo identificador.
//
// Coluna é o caso que obrigou: tabela com nome ruim tem saída pelo `.Alias()`, que
// existe para isso; coluna não tem apelido nenhum.
func TestNomeFisicoAceitaOQueOsBancosAceitam(t *testing.T) {
	validos := []string{
		"users", "eventos_bkp435037", "data_", "autenticação", "carta_concessão",
		"linkarportaltransparência", "a", "tabela__dupla", "col1",
	}
	for _, nome := range validos {
		if !NomeFisicoValido(nome) {
			t.Errorf("%q deveria ser aceito: funciona nos quatro bancos", nome)
		}
	}
}

// O que ela continua recusando é o que quebra ou engana — cada caso com sua razão.
func TestNomeFisicoRecusaOQueQuebra(t *testing.T) {
	invalidos := map[string]string{
		"minha coluna":  "espaço quebra SQL escrito à mão",
		"<idx_algo>":    "símbolo; existe em banco legado por descuido",
		"idx-algo":      "hífen é operador",
		"Autenticacao":  "MAIÚSCULA: o Oracle dobra a caixa e o MySQL em Linux a diferencia",
		"AUTENTICACAO":  "idem",
		"2fa":           "começar por dígito é recusado por parte dos bancos",
		"_interno":      "começar por underscore",
		"":              "vazio",
		"tabela.coluna": "ponto é separador de qualificação",
	}
	for nome, razao := range invalidos {
		if NomeFisicoValido(nome) {
			t.Errorf("%q deveria ser recusado (%s)", nome, razao)
		}
	}
}
