package orm

// Classificação de erro de banco.
//
// Sem isso, quem consome a ORM só tem duas informações: deu certo ou deu erro
// 500. Chave duplicada, violação de FK e coluna obrigatória vazia são as três
// falhas que a aplicação precisa distinguir — a primeira é 409, as outras são
// 422, e nenhuma delas é culpa do servidor.
//
// A classificação é por dialeto porque a mensagem é. O gokit já fazia esse
// reconhecimento na camada de factory para dar conselho ao desenvolvedor; aqui
// o mesmo conhecimento vira erro tipado, disponível para a camada HTTP.

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinelas de classe. Use errors.Is para testar.
var (
	// ErrDuplicate é violação de chave única ou primária.
	ErrDuplicate = errors.New("orm: valor duplicado em coluna única")
	// ErrForeignKey é violação de chave estrangeira, nos dois sentidos: inserir
	// filho sem pai, ou remover pai que ainda tem filho.
	ErrForeignKey = errors.New("orm: violação de chave estrangeira")
	// ErrNotNull é coluna obrigatória sem valor.
	ErrNotNull = errors.New("orm: coluna obrigatória sem valor")
	// ErrCheck é violação de CHECK constraint.
	ErrCheck = errors.New("orm: valor recusado por restrição CHECK")
	// ErrTooLong é valor maior que o tamanho declarado da coluna.
	ErrTooLong = errors.New("orm: valor maior que o tamanho da coluna")
)

// DatabaseError embrulha o erro do driver com a classe reconhecida. O erro
// original continua acessível por errors.Unwrap, porque a mensagem do banco é
// o que resolve o caso difícil.
type DatabaseError struct {
	Classe error // um dos sentinelas acima
	Causa  error
}

func (e DatabaseError) Error() string {
	if e.Classe == nil {
		return e.Causa.Error()
	}
	return fmt.Sprintf("%s: %s", e.Classe.Error(), e.Causa.Error())
}

// Is faz errors.Is(err, ErrDuplicate) funcionar, e também
// errors.Is(err, causaOriginal).
func (e DatabaseError) Is(alvo error) bool {
	return e.Classe != nil && errors.Is(e.Classe, alvo)
}

func (e DatabaseError) Unwrap() error { return e.Causa }

// marcadores mapeia trechos de mensagem para a classe. São os mesmos dos quatro
// drivers, em minúsculas.
//
// A ordem importa: "duplicate key value violates unique constraint" do Postgres
// contém "violates", e "violates foreign key" também — então o mais específico
// precisa ser testado primeiro. Por isso é fatia ordenada, e não mapa.
var marcadores = []struct {
	trecho string
	classe error
}{
	// Duplicidade.
	{"duplicate entry", ErrDuplicate},             // MySQL 1062
	{"duplicate key value", ErrDuplicate},         // Postgres 23505
	{"unique constraint", ErrDuplicate},           // Oracle ORA-00001 / Postgres
	{"ora-00001", ErrDuplicate},                   // Oracle explícito
	{"cannot insert duplicate key", ErrDuplicate}, // SQL Server 2601/2627
	{"violation of primary key", ErrDuplicate},    // SQL Server
	{"violation of unique key", ErrDuplicate},     // SQL Server

	// Chave estrangeira.
	{"foreign key", ErrForeignKey},                    // os quatro
	{"ora-02291", ErrForeignKey},                      // Oracle: pai não existe
	{"ora-02292", ErrForeignKey},                      // Oracle: filho ainda existe
	{"a foreign key constraint fails", ErrForeignKey}, // MySQL 1451/1452

	// Obrigatoriedade.
	{"cannot be null", ErrNotNull},               // MySQL 1048
	{"null value in column", ErrNotNull},         // Postgres 23502
	{"ora-01400", ErrNotNull},                    // Oracle
	{"cannot insert the value null", ErrNotNull}, // SQL Server 515
	{"not-null constraint", ErrNotNull},          // Postgres

	// CHECK.
	{"check constraint", ErrCheck},
	{"ora-02290", ErrCheck},

	// Tamanho.
	{"data too long", ErrTooLong},                            // MySQL 1406
	{"value too long", ErrTooLong},                           // Postgres 22001
	{"ora-12899", ErrTooLong},                                // Oracle
	{"string or binary data would be truncated", ErrTooLong}, // SQL Server
}

// ClassifyError reconhece a classe de um erro de banco e o embrulha. Erro que
// não casa com nenhuma classe volta como está: inventar classe é pior que
// admitir que não sabe.
//
// É exportada porque a camada de resposta HTTP precisa dela, e porque quem
// escreve SQL específico de dialeto pela mão também merece a classificação.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	var jaClassificado DatabaseError
	if errors.As(err, &jaClassificado) {
		return err
	}
	mensagem := strings.ToLower(err.Error())
	for _, marcador := range marcadores {
		if strings.Contains(mensagem, marcador.trecho) {
			return DatabaseError{Classe: marcador.classe, Causa: err}
		}
	}
	return err
}
