package orm

// Atribuição tipada: o valor de uma escrita checado pelo compilador.
//
// `Values` é `map[Column]any`, então o valor não é checado por ninguém:
//
//	orm.Values{f.Nome: 123, f.Ativo: "talvez"}   // compila, falha no banco
//
// O gerador já sabe o tipo de cada coluna — é ele que emite StringColumn,
// NumberColumn, BoolColumn e DateColumn. Aqui esse conhecimento passa a valer na
// autoria: o tipo da coluna decide o que o método aceita, e o erro vira de
// compilação.
//
// A tipagem segue exatamente a que os FILTROS já adotaram, e não é acidente:
// texto e booleano são tipados; número aceita int e float sem conversão na
// autoria; data aceita as formas que DateValue resolve (time.Time, ponteiro,
// Value, texto em formato conhecido). Divergir disso deixaria a escrita mais
// restrita que o filtro, sem ganho — e Go não permite parâmetro de tipo em
// método, então uma restrição numérica genérica não está disponível.
//
// Values continua existindo para o caso dinâmico, em que a lista de colunas é
// decidida em runtime (importação, formulário genérico). É a mesma relação do
// Record.ByName com os getters tipados: a saída existe, e o nome diz que ali se
// abre mão da checagem.

import "fmt"

// Assignment é uma coluna com o valor a gravar.
type Assignment struct {
	campo Field
	valor any
	// erro carrega falha de autoria detectada no construtor (SetNull em coluna
	// obrigatória, por exemplo). Não dá para devolver erro de uma expressão que
	// vive dentro de uma chamada, então ele viaja até o Set.
	erro error
}

// Set reúne as atribuições de uma escrita.
//
// Devolve Values, então Insert, Update, Upsert e InsertIgnore recebem sem
// nenhuma mudança de assinatura — a forma tipada é uma porta de entrada nova para
// a mesma estrutura, não uma segunda estrutura.
func Set(atribuicoes ...Assignment) Values {
	valores := make(Values, len(atribuicoes))
	vistas := make(map[string]bool, len(atribuicoes))
	for _, atribuicao := range atribuicoes {
		if atribuicao.erro != nil {
			return valoresComErro(atribuicao.erro)
		}
		// Coluna repetida: o mapa colapsaria em silêncio, ficando com a última —
		// e num INSERT isso é o valor errado gravado sem aviso. Aqui a lista ainda
		// está inteira, então dá para recusar. É a única coisa que a forma tipada
		// consegue pegar e a literal de mapa não.
		chave := atribuicao.campo.Table + "." + atribuicao.campo.Column
		if vistas[chave] {
			return valoresComErro(errColunaRepetida(atribuicao.campo.Column))
		}
		vistas[chave] = true
		valores[atribuicao.campo] = atribuicao.valor
	}
	return valores
}

// colunaDeErro é a chave sentinela que transporta um erro de autoria dentro do
// Values. Values é um mapa, então não tem onde guardar erro; e entrar em panic
// no meio de uma expressão de autoria seria pior que atrasar a falha até o
// terminal, que é onde todo outro erro de escrita já aparece.
//
// O nome começa com um byte nulo, que nome de coluna nunca tem.
var colunaDeErro = Field{Column: "\x00erro"}

func valoresComErro(err error) Values {
	return Values{colunaDeErro: err}
}

// conferirAutoria é chamada no COMEÇO de cada terminal de escrita, antes de
// resolver a conexão.
//
// A ordem importa para o diagnóstico: erro de autoria é determinístico e não
// depende de dialeto, enquanto "nenhuma conexão ativa" é ambiental. Resolvendo a
// conexão primeiro, uma coluna repetida aparecia como erro de conexão — e manda
// quem escreveu investigar a coisa errada.
func conferirAutoria(linhas ...Values) error {
	for _, valores := range linhas {
		if err := valores.erroDeAutoria(); err != nil {
			return err
		}
	}
	return nil
}

// erroDeAutoria devolve o erro transportado, se houver.
func (v Values) erroDeAutoria() error {
	bruto, tem := v[colunaDeErro]
	if !tem {
		return nil
	}
	if err, ok := bruto.(error); ok {
		return err
	}
	return nil
}

// ── Atribuição por tipo de coluna ──

func (f StringColumn) Is(v string) Assignment { return Assignment{campo: f.Field, valor: v} }
func (f BoolColumn) Is(v bool) Assignment     { return Assignment{campo: f.Field, valor: v} }

// Is de número aceita int, int64 e float sem conversão na autoria — a mesma
// escolha do NumberColumn.Equal.
func (f NumberColumn) Is(v any) Assignment { return Assignment{campo: f.Field, valor: v} }

// Is de data passa pelo DateValue, então time.Time, ponteiro, Value e texto em
// formato conhecido chegam ao driver já como time.Time.
func (f DateColumn) Is(v any) Assignment {
	return Assignment{campo: f.Field, valor: DateValue(v)}
}

// SetNull grava NULL.
//
// Só faz sentido em coluna que aceita nulo, e por isso a coluna obrigatória falha
// aqui em vez de no banco: `Values{f.Nome: nil}` só quebra no INSERT, com o erro
// do driver, e nunca é intenção legítima — quem quer o padrão da coluna omite a
// coluna.
func (f StringColumn) SetNull() Assignment { return atribuirNulo(f.Field) }
func (f NumberColumn) SetNull() Assignment { return atribuirNulo(f.Field) }
func (f BoolColumn) SetNull() Assignment   { return atribuirNulo(f.Field) }
func (f DateColumn) SetNull() Assignment   { return atribuirNulo(f.Field) }

func atribuirNulo(campo Field) Assignment {
	if !campo.Nullable {
		return Assignment{erro: errNuloEmColunaObrigatoria(campo.Column)}
	}
	return Assignment{campo: campo, valor: nil}
}

func errColunaRepetida(coluna string) error {
	return fmt.Errorf("orm: a coluna %s foi atribuída duas vezes na mesma escrita; a segunda sobrescreveria a primeira em silêncio", coluna)
}

func errNuloEmColunaObrigatoria(coluna string) error {
	return fmt.Errorf("orm: a coluna %s não aceita nulo, então SetNull() nela nunca é a intenção; para deixar o banco aplicar o padrão, omita a coluna da escrita", coluna)
}
