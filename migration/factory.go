package migrate

// Ruler controla como a factory é executada.
//
//	Count  quantas linhas gerar (0 é tratado como 1)
//	Active se esta factory entra nas execuções em lote
//
// Active é por factory, não por arquivo: cada função tem o seu, mesmo quando
// várias moram no mesmo arquivo. Serve para a tabela que a migration cria mas
// você não quer popular.
//
// Não existe mais um campo para congelar o arquivo contra a regeneração: a lista
// de colunas é sempre acertada contra a migration, porque uma factory que cita
// coluna inexistente só falharia no banco. O que você escreveu em cada coluna
// continua preservado.
type Ruler struct {
	Count  int
	Active bool
}

// Fields é uma linha gerada: coluna -> valor.
//
// A chave é ColumnName, não string: literal continua compilando (constante sem
// tipo converte), a referência de catálogo compila, e nome de coluna vindo de
// VARIÁVEL deixa de compilar — que é o ponto.
type Fields = map[ColumnName]any

// Factory descreve como popular uma tabela com dados fake.
//
//	func CidadesFactory() migrate.Factory {
//	    return migrate.Factory{
//	        Table: core.Table.Cidades,
//	        Ruler: migrate.Ruler{Count: 10, Active: true},
//	        Data: migrate.Fields{
//	            core.Column.Cidades.CidadeId: migrate.FakeInt(1, 999999999),
//	            core.Column.Cidades.Nome:     migrate.FakeName(75),
//	        },
//	    }
//	}
//
// Data é lido por AST e nunca compilado pelo gokit: só as funções Fake*,
// Reference, Seeder e literais são aceitos ali.
//
// Era uma closure `func(index int) Fields`, porque o arquivo escrevia `index`
// dentro do mapa e o parâmetro precisava existir para o arquivo ser Go válido.
// Depois que o índice virou implícito, ninguém mais o escreve — e a closure
// passou a ser só um invólucro sem função para o compilador nem para o motor.
type Factory struct {
	Table Table
	Ruler Ruler
	Data  Fields
}
