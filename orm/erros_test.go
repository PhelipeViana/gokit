package orm

import (
	"errors"
	"fmt"
	"testing"
)

// As mensagens abaixo são as que os quatro drivers realmente devolvem. A
// classificação existe para que a camada HTTP responda 409 ou 422 sem inspecionar
// texto — e para que ela acerte, os trechos precisam bater com o mundo real.
func TestClassificacaoDosErrosDosQuatroBancos(t *testing.T) {
	casos := []struct {
		banco    string
		mensagem string
		classe   error
	}{
		{"mysql", "Error 1062 (23000): Duplicate entry 'ana@exemplo.com' for key 'users.email'", ErrDuplicate},
		{"postgres", `ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)`, ErrDuplicate},
		{"oracle", "ORA-00001: unique constraint (GOKIT.UK_USERS_EMAIL) violated", ErrDuplicate},
		{"sqlserver", "mssql: Violation of UNIQUE KEY constraint 'UK_users_email'. Cannot insert duplicate key in object 'dbo.users'.", ErrDuplicate},

		{"mysql", "Error 1452 (23000): Cannot add or update a child row: a foreign key constraint fails", ErrForeignKey},
		{"postgres", `ERROR: insert or update on table "users" violates foreign key constraint "fk_users_cidade" (SQLSTATE 23503)`, ErrForeignKey},
		{"oracle", "ORA-02291: integrity constraint (GOKIT.FK_USERS_CIDADE) violated - parent key not found", ErrForeignKey},
		{"sqlserver", `mssql: The INSERT statement conflicted with the FOREIGN KEY constraint "FK_users_cidade".`, ErrForeignKey},

		{"mysql", "Error 1048 (23000): Column 'nome' cannot be null", ErrNotNull},
		{"postgres", `ERROR: null value in column "nome" of relation "users" violates not-null constraint (SQLSTATE 23502)`, ErrNotNull},
		{"oracle", `ORA-01400: cannot insert NULL into ("GOKIT"."USERS"."NOME")`, ErrNotNull},
		{"sqlserver", "mssql: Cannot insert the value NULL into column 'nome', table 'gokit.dbo.users'", ErrNotNull},

		{"mysql", "Error 3819 (HY000): Check constraint 'chk_users_idade' is violated.", ErrCheck},
		{"oracle", "ORA-02290: check constraint (GOKIT.CHK_USERS_IDADE) violated", ErrCheck},

		{"mysql", "Error 1406 (22001): Data too long for column 'nome' at row 1", ErrTooLong},
		{"postgres", "ERROR: value too long for type character varying(30) (SQLSTATE 22001)", ErrTooLong},
		{"oracle", "ORA-12899: value too large for column \"GOKIT\".\"USERS\".\"NOME\"", ErrTooLong},
		{"sqlserver", "mssql: String or binary data would be truncated in table 'gokit.dbo.users'", ErrTooLong},
	}

	for _, caso := range casos {
		classificado := ClassifyError(errors.New(caso.mensagem))
		if !errors.Is(classificado, caso.classe) {
			t.Errorf("%s: esperado %v\n  mensagem: %s\n  obtido:   %v",
				caso.banco, caso.classe, caso.mensagem, classificado)
		}
	}
}

// A duplicidade do Postgres traz "violates unique constraint", e a de FK traz
// "violates foreign key constraint". Se a ordem dos marcadores estiver errada, um
// vira o outro — e o cliente recebe 422 onde devia receber 409.
func TestDuplicidadeNaoEConfundidaComForeignKey(t *testing.T) {
	duplicada := ClassifyError(errors.New(
		`ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)`))
	if !errors.Is(duplicada, ErrDuplicate) {
		t.Errorf("deveria ser ErrDuplicate: %v", duplicada)
	}
	if errors.Is(duplicada, ErrForeignKey) {
		t.Error("não deveria ser classificada como ErrForeignKey")
	}
}

// Erro que não é de restrição volta como está: inventar classe esconderia a causa
// real de um problema de infraestrutura.
func TestErroDesconhecidoNaoEClassificado(t *testing.T) {
	original := errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")
	classificado := ClassifyError(original)
	if classificado != original {
		t.Errorf("erro desconhecido deveria voltar intacto, veio: %v", classificado)
	}
	var comoBanco DatabaseError
	if errors.As(classificado, &comoBanco) {
		t.Error("não deveria ter sido embrulhado em DatabaseError")
	}
}

// A causa original precisa continuar acessível: é a mensagem do banco que resolve
// o caso difícil, e a classe é só o resumo.
func TestCausaOriginalContinuaAcessivel(t *testing.T) {
	original := errors.New("Error 1062 (23000): Duplicate entry 'x' for key 'users.email'")
	classificado := ClassifyError(original)

	if !errors.Is(classificado, ErrDuplicate) {
		t.Fatalf("classificação falhou: %v", classificado)
	}
	if !errors.Is(classificado, original) {
		t.Error("errors.Is deveria alcançar a causa original")
	}
	if desembrulhado := errors.Unwrap(classificado); desembrulhado != original {
		t.Errorf("Unwrap deveria devolver a causa, veio: %v", desembrulhado)
	}
}

func TestClassificarDuasVezesNaoEmbrulhaDeNovo(t *testing.T) {
	uma := ClassifyError(errors.New("Duplicate entry 'x' for key 'users.email'"))
	duas := ClassifyError(uma)
	if fmt.Sprint(uma) != fmt.Sprint(duas) {
		t.Errorf("classificação não é idempotente:\n  %v\n  %v", uma, duas)
	}
}

func TestClassifyErrorNilVoltaNil(t *testing.T) {
	if ClassifyError(nil) != nil {
		t.Error("nil deveria voltar nil")
	}
}
