package db

// Transação.
//
// Escrita que toca mais de uma tabela precisa ser atômica, e até aqui a única
// forma era pegar o *sql.DB embutido e cuidar de tudo à mão. O Tx entrega um
// Runner que a ORM aceita nos métodos *With, então a autoria dentro da transação
// é idêntica à de fora.

import (
	"context"
	"database/sql"
	"errors"

	"github.com/PhelipeViana/gokit/orm"
)

// TxSession é uma transação em curso que satisfaz orm.Runner e sabe escrever.
type TxSession struct {
	*sql.Tx
	dialect orm.Dialect
	schema  string
}

func (t *TxSession) Dialect() orm.Dialect { return t.dialect }
func (t *TxSession) Schema() string       { return t.schema }

// Tx roda fn dentro de uma transação: commit se fn devolver nil, rollback caso
// contrário.
//
// O rollback também acontece em panic, e o panic é repassado — engolir panic
// deixaria a aplicação seguir com estado inconsistente, que é pior que o panic.
func (s *Session) Tx(ctx context.Context, fn func(r orm.Runner) error) error {
	transacao, err := s.BeginTx(ctx, nil)
	if err != nil {
		return orm.ClassifyError(err)
	}
	sessao := &TxSession{Tx: transacao, dialect: s.dialect, schema: s.schema}

	defer func() {
		if recuperado := recover(); recuperado != nil {
			_ = transacao.Rollback()
			panic(recuperado)
		}
	}()

	if err := fn(sessao); err != nil {
		// O erro do rollback não substitui o erro original: quem chamou precisa
		// saber por que a transação foi desfeita, não que o desfazer também falhou.
		_ = transacao.Rollback()
		return err
	}
	if err := transacao.Commit(); err != nil {
		return orm.ClassifyError(err)
	}
	return nil
}

// Attempt roda fn dentro de um SAVEPOINT: se fn falhar, desfaz só o que ela fez e
// a transação continua utilizável.
//
// Existe por uma divergência real entre os bancos. No Postgres, qualquer erro
// aborta a transação inteira — todo comando seguinte falha com "current
// transaction is aborted" (25P02). MySQL, Oracle e SQL Server seguem aceitando
// comandos. Sem savepoint, um service que capture ErrDuplicate e continue funciona
// em três dialetos e quebra em um, que é exatamente o tipo de diferença que a ORM
// promete não ter.
//
// O erro de fn é devolvido como está, para que errors.Is(err, orm.ErrDuplicate)
// continue funcionando em quem chamou.
func (t *TxSession) Attempt(ctx context.Context, nome string, fn func(r orm.Runner) error) error {
	// O nome entra no SQL, então só identificador simples é aceito: savepoint não
	// aceita parâmetro, e concatenar texto de fora seria injeção.
	if !nomeDeSavepointValido(nome) {
		return orm.ErrInvalidSavepoint
	}
	criar, voltar := comandosDeSavepoint(t.dialect, nome)
	if _, err := t.ExecContext(ctx, criar); err != nil {
		return orm.ClassifyError(err)
	}
	if err := fn(t); err != nil {
		if _, voltaErr := t.ExecContext(ctx, voltar); voltaErr != nil {
			// Falhar ao voltar deixa a transação em estado desconhecido; o erro
			// original continua sendo o que importa, mas o segundo não pode sumir.
			return errors.Join(err, voltaErr)
		}
		return err
	}
	// RELEASE não existe no Oracle nem no SQL Server, e nos dois o savepoint morre
	// com a transação — então liberar é otimização, não obrigação.
	if t.dialect == orm.Postgres || t.dialect == orm.MySQL {
		_, _ = t.ExecContext(ctx, "RELEASE SAVEPOINT "+nome)
	}
	return nil
}

// comandosDeSavepoint devolve o par criar/voltar de cada dialeto.
//
// O SQL Server não usa a sintaxe padrão: lá é SAVE TRANSACTION e ROLLBACK
// TRANSACTION. Os outros três aceitam SAVEPOINT / ROLLBACK TO SAVEPOINT.
func comandosDeSavepoint(dialeto orm.Dialect, nome string) (criar, voltar string) {
	if dialeto == orm.SQLServer {
		return "SAVE TRANSACTION " + nome, "ROLLBACK TRANSACTION " + nome
	}
	return "SAVEPOINT " + nome, "ROLLBACK TO SAVEPOINT " + nome
}

// nomeDeSavepointValido aceita apenas letra, dígito e sublinhado, começando por
// letra — o subconjunto que os quatro bancos aceitam como identificador.
func nomeDeSavepointValido(nome string) bool {
	if nome == "" || len(nome) > 30 {
		return false
	}
	for i, r := range nome {
		letra := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digito := r >= '0' && r <= '9'
		if i == 0 && !letra {
			return false
		}
		if !letra && !digito && r != '_' {
			return false
		}
	}
	return true
}
