package migraterun

// Guarda do filtro de coluna de INCLUDE na leitura de índices do SQL Server.
//
// Coluna de INCLUDE não faz parte da CHAVE do índice — é carga de cobertura, e vem do
// catálogo com `key_ordinal = 0`. O DSL do gokit não tem INCLUDE, então sem o filtro a
// coluna entrava na lista de chave e o corpus declarava um índice que NENHUM dos quatro
// bancos aceita: `IDX_FOLHA_PROC_ANOMESGRUPO` tem 1 coluna de chave e 57 de INCLUDE, e saía
// como 58 colunas de chave contra um teto de 32.
//
// O teste é sobre o TEXTO da consulta porque é o único jeito de guardar isto sem um SQL
// Server de pé. Se um dia houver, o que vale medir é o número de colunas daquele índice.

import (
	"strings"
	"testing"
)

func TestConsultaDeIndicesDoSQLServerIgnoraColunaDeInclude(t *testing.T) {
	comando, _ := consultaDeIndices("sqlserver", "dbo")
	if comando == "" {
		t.Fatal("consulta do sqlserver vazia")
	}
	if !strings.Contains(comando, "is_included_column = 0") {
		t.Errorf("a consulta precisa excluir coluna de INCLUDE da chave:\n%s", comando)
	}
}

// Os outros três não têm o conceito de INCLUDE, então não devem carregar o filtro — se
// aparecer ali, é copiar-e-colar e a consulta quebra.
func TestSomenteOSQLServerFiltraColunaDeInclude(t *testing.T) {
	for _, dialeto := range []string{"mysql", "postgres", "oracle"} {
		if strings.Contains(consultaDeIndicesTexto(dialeto), "is_included_column") {
			t.Errorf("%s não tem coluna de INCLUDE; o filtro não deveria estar na consulta", dialeto)
		}
	}
}

func consultaDeIndicesTexto(dialeto string) string {
	comando, _ := consultaDeIndices(dialeto, "esquema")
	return comando
}
