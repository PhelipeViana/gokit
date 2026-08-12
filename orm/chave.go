package orm

// Modelo de chave primária.
//
// A escrita precisa saber identificar uma linha, e o `id` int auto-incremento é
// só o caso mais comum. Schema legado — que é para onde o gokit vai — vive de
// chave com outro nome e de chave composta. A informação já vem da migration no
// fields.gen.go (PrimaryKey por coluna); aqui ela é lida.
//
// A ordem das colunas é a de declaração na entidade, não alfabética: é a mesma
// ordem que o CreateTable escreveu, então `Find(ctx, 7, "A")` casa com o que o
// desenvolvedor lê na migration.

import "strings"

// PrimaryKeys devolve as colunas de chave primária na ordem declarada.
func (e EntityFields) PrimaryKeys() []Field {
	var chaves []Field
	for _, field := range e.Fields {
		if field.PrimaryKey {
			chaves = append(chaves, field)
		}
	}
	return chaves
}

// autoIncrement devolve a coluna auto-incremento, se houver. É ela que o banco
// preenche no INSERT e que precisa voltar para quem inseriu.
func (e EntityFields) autoIncrement() (Field, bool) {
	for _, field := range e.Fields {
		if field.AutoIncrement {
			return field, true
		}
	}
	return Field{}, false
}

// fieldPorColuna acha o Field de uma coluna pelo nome, ignorando caixa — cada
// banco normaliza identificador de um jeito.
func (e EntityFields) fieldPorColuna(coluna string) (Field, bool) {
	for _, field := range e.Fields {
		if strings.EqualFold(field.Column, coluna) {
			return field, true
		}
	}
	return Field{}, false
}

// condicaoPorChave monta a condição que identifica uma linha pela chave
// primária. Aceita os valores na ordem declarada.
//
// Devolve erro em vez de silenciosamente filtrar por menos colunas: identificar
// linha errada num UPDATE é pior que falhar.
func (e EntityFields) condicaoPorChave(valores []any) (Expression, error) {
	chaves := e.PrimaryKeys()
	if len(chaves) == 0 {
		return nil, errSemChavePrimaria(e.Name)
	}
	if len(valores) != len(chaves) {
		return nil, errChaveIncompleta(e.Name, len(chaves), len(valores), nomesDeColunas(chaves))
	}
	condicoes := make([]Expression, 0, len(chaves))
	for i, chave := range chaves {
		condicoes = append(condicoes, condition(chave, Equal, valores[i]))
	}
	if len(condicoes) == 1 {
		return condicoes[0], nil
	}
	return And(condicoes...), nil
}

func nomesDeColunas(campos []Field) string {
	nomes := make([]string, 0, len(campos))
	for _, campo := range campos {
		nomes = append(nomes, campo.Column)
	}
	return strings.Join(nomes, ", ")
}
