package migrate

// Ruler controla como a factory é executada.
//
//	Count  quantas linhas gerar (0 é tratado como 1)
//	Update se o gokit pode reescrever este arquivo quando a tabela mudar
//	Active se a factory entra nas execuções em lote
type Ruler struct {
	Count  int
	Update bool
	Active bool
}

// Fields é uma linha gerada: coluna -> valor.
type Fields = map[string]any

// Factory descreve como popular uma tabela com dados fake.
//
//	func CidadesFactory() migrate.Factory {
//	    return migrate.Factory{
//	        Table: "CIDADES",
//	        Ruler: migrate.Ruler{Count: 10, Update: true, Active: true},
//	        Data: func(index int) migrate.Fields {
//	            return migrate.Fields{
//	                "CIDADE_ID": migrate.FakeIntIndex(index, 1, 999999999),
//	                "NOME":      migrate.FakeNameIndexLength(index, 75),
//	            }
//	        },
//	    }
//	}
//
// O corpo de Data é lido por AST e nunca compilado pelo gokit: só as funções
// Fake*, Vinculo e literais são aceitos ali. A closure existe para dar um
// significado a index — o arquivo continua sendo Go válido, e o editor
// autocompleta normalmente.
type Factory struct {
	Table string
	Ruler Ruler
	Data  func(index int) Fields
}
