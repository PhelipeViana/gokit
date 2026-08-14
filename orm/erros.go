package orm

// Classificação de erro de banco.
//
// Sem isso, quem consome a ORM só tem duas informações: deu certo ou deu erro
// 500. Duplicidade é 409; FK, obrigatoriedade, CHECK, tamanho, tipo e faixa são
// 422; e nenhuma delas é culpa do servidor. O statusOf do response.gen.go é quem
// faz essa tradução — classificar sem ninguém consumir a classe não muda nada para
// quem chama a API, e por um tempo foi exatamente esse o caso.
//
// A classificação é por dialeto porque a mensagem é. O gokit já fazia esse
// reconhecimento na camada de factory para dar conselho ao desenvolvedor; aqui
// o mesmo conhecimento vira erro tipado, disponível para a camada HTTP.
//
// A tabela de marcadores é MEDIDA, não escrita de cabeça: a sonda em
// internal/migraterun/sonda_erros_test.go provoca cada classe nos quatro bancos e
// confere o veredito contra a mensagem crua do driver. Marcador novo entra por ali —
// foi assim que apareceu o "reference constraint" do SQL Server, que ninguém
// adivinharia. Rode com GOKIT_SONDA=1 e os containers de pé.

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
	// ErrInvalidValue é valor que não converte para o tipo da coluna: texto em
	// coluna numérica, data mal formada. É dado do cliente, não falha do servidor.
	ErrInvalidValue = errors.New("orm: valor incompatível com o tipo da coluna")
	// ErrOutOfRange é número que cabe no tipo Go e não na precisão declarada da
	// coluna. Separado do ErrInvalidValue porque o conserto é outro: ali o valor está
	// malformado, aqui está bem formado e é grande demais.
	ErrOutOfRange = errors.New("orm: valor fora da faixa da coluna")
	// ErrUndefinedObject é tabela ou coluna que não existe no banco. Vindo da ORM
	// gerada isso não acontece — o SQL sai do catálogo. Aparece com SQL cru e com
	// migration pendente, e nos dois casos é erro de programa, não de dado: fica
	// classificado para poder ser distinguido de falha de infraestrutura.
	ErrUndefinedObject = errors.New("orm: tabela ou coluna inexistente")
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
	// O SQL Server troca a palavra conforme o comando: INSERT bloqueado diz "FOREIGN
	// KEY constraint", e DELETE bloqueado diz "REFERENCE constraint" — a mesma
	// violação com outro nome. Sem esta linha, apagar pai com filho voltava sem classe
	// nesse dialeto e só nele. Medido em 2026-08-14 pela sonda.
	{"reference constraint", ErrForeignKey}, // SQL Server 547 no DELETE/UPDATE

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

	// Faixa numérica. Vem antes da conversão porque as duas mensagens do Oracle e do
	// SQL Server falam de "convert"/"converting": "arithmetic overflow error
	// converting expression" é estouro, não valor malformado.
	{"out of range value for column", ErrOutOfRange},    // MySQL 1264
	{"out of range", ErrOutOfRange},                     // Postgres 22003 ("integer out of range")
	{"ora-01438", ErrOutOfRange},                        // Oracle: maior que a precisão
	{"greater than specified precision", ErrOutOfRange}, // Oracle, sem o código
	{"arithmetic overflow", ErrOutOfRange},              // SQL Server 8115

	// Conversão de tipo.
	{"incorrect integer value", ErrInvalidValue},           // MySQL 1366
	{"incorrect decimal value", ErrInvalidValue},           // MySQL
	{"incorrect datetime value", ErrInvalidValue},          // MySQL
	{"invalid input syntax for type", ErrInvalidValue},     // Postgres 22P02
	{"ora-01722", ErrInvalidValue},                         // Oracle: não é número
	{"ora-01858", ErrInvalidValue},                         // Oracle: não é data
	{"conversion failed when converting", ErrInvalidValue}, // SQL Server 245

	// Objeto inexistente. Fica no FIM da lista de propósito: os trechos são os mais
	// genéricos de todos ("does not exist"), e qualquer mensagem de restrição que os
	// contenha precisa ter casado antes.
	{"ora-00942", ErrUndefinedObject},           // Oracle: tabela ou view
	{"ora-00904", ErrUndefinedObject},           // Oracle: identificador inválido
	{"unknown column", ErrUndefinedObject},      // MySQL 1054
	{"doesn't exist", ErrUndefinedObject},       // MySQL 1146
	{"invalid object name", ErrUndefinedObject}, // SQL Server 208
	{"invalid column name", ErrUndefinedObject}, // SQL Server 207
	{"does not exist", ErrUndefinedObject},      // Postgres 42P01 / 42703
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
