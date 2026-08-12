package orm

// Avaliação do critério em memória.
//
// O mesmo critério que filtra no banco passa a responder sobre uma linha que já
// está carregada:
//
//	criterio := u.Field.Nome.Contains("ana")
//	orm.Users.Where(criterio).Get(ctx)   // o banco avalia
//	if criterio.Matches(linha) { ... }    // o Go avalia, sem ida ao banco
//
// A razão de existir é a camada de regras: escrever a regra duas vezes — uma como
// cláusula e outra como if — deixa as duas divergirem sem ninguém notar.
//
// A leitura do valor usa reflexão sobre o Field.Name, que é exatamente o nome do
// campo Go na struct gerada. Custou zero mudança no gerador, e o custo de
// desempenho é irrelevante: o alternativo a esta avaliação é uma ida ao banco,
// que é milhares de vezes mais caro que uma reflexão.

import (
	"reflect"
	"strings"
	"time"
)

// Matches responde se a linha satisfaz a condição.
//
// Coluna que não existe na linha devolve falso — é o mesmo desfecho de uma linha
// que não satisfaz, e é o que faz projeção parcial (.Select) não virar panic.
func (f Condition) Matches(linha any) bool {
	if !f.Active {
		// Condição inativa não filtra nada no SQL; em memória, o equivalente é
		// não excluir a linha.
		return true
	}
	valor, ok := valorDaLinha(linha, f.Field)
	if !ok {
		return false
	}
	return avaliar(valor, f.Operator, f.Values)
}

// Matches avalia o grupo respeitando o conector de cada item, na mesma ordem em
// que o SQL avaliaria.
func (g Group) Matches(linha any) bool {
	resultado := true
	primeiro := true
	for _, it := range g.items {
		if it.expr == nil || !it.expr.active() {
			continue
		}
		parcial := Matches(it.expr, linha)
		if primeiro {
			resultado = parcial
			primeiro = false
			continue
		}
		if it.or {
			resultado = resultado || parcial
		} else {
			resultado = resultado && parcial
		}
	}
	return resultado
}

// Matches avalia qualquer expressão. Serve para quem tem uma Expression na mão
// sem saber o tipo concreto — o caso de uma regra guardada numa variável.
//
// Expressão que depende do banco (o EXISTS de uma relação) não tem resposta em
// memória e devolve falso. Use Evaluable antes quando isso importar.
func Matches(expressao Expression, linha any) bool {
	switch v := expressao.(type) {
	case Condition:
		return v.Matches(linha)
	case Group:
		return v.Matches(linha)
	case exprCondition:
		return v.Matches(linha)
	case Raw:
		// Fragmento cru é SQL: não há como o Go respondê-lo. Devolver falso, e
		// Evaluable dizer falso antes, é o que permite a camada de regras saber que
		// aquela regra só o banco sabe avaliar.
		_ = v
		return false
	default:
		return false
	}
}

// Matches avalia a condição sobre expressão em memória.
//
// Existe para que o bloco 5 não fure a promessa do Matches: uma regra guardada
// numa variável tem de responder igual no banco e no Go, e sem isto
// orm.Lower(f.Nome).Contains("ana") passaria a ser uma regra que só o banco sabe
// avaliar — e a camada de regras voltaria a escrever a mesma condição duas vezes.
func (c exprCondition) Matches(linha any) bool {
	if !c.active() {
		return true
	}
	valor, ok := avaliarExpr(c.expr, linha)
	if !ok {
		return false
	}
	return avaliar(valor, c.op, c.values)
}

// avaliarExpr calcula a expressão sobre uma linha carregada. Devolve falso quando
// não dá para calcular — coluna ausente da projeção, ou função sem equivalente em
// memória.
func avaliarExpr(e Expr, linha any) (any, bool) {
	if e.fn == fnNow {
		// O relógio do Go, não o do banco. É a única função do bloco cuja avaliação
		// em memória não pode ser idêntica à do SQL, e está documentado no Now().
		return time.Now(), true
	}

	entradas := make([]any, 0, len(e.args))
	for _, arg := range e.args {
		if arg.eValor {
			entradas = append(entradas, arg.valor)
			continue
		}
		if aninhada, ok := arg.coluna.(Expr); ok {
			valor, ok := avaliarExpr(aninhada, linha)
			if !ok {
				return nil, false
			}
			entradas = append(entradas, valor)
			continue
		}
		valor, ok := valorDaLinha(linha, arg.coluna.columnField())
		if !ok {
			return nil, false
		}
		entradas = append(entradas, valor)
	}
	if len(entradas) == 0 {
		return nil, false
	}

	switch e.fn {
	case fnLower:
		return strings.ToLower(comoTexto(entradas[0])), true
	case fnUpper:
		return strings.ToUpper(comoTexto(entradas[0])), true
	case fnTrim:
		return strings.TrimSpace(comoTexto(entradas[0])), true
	case fnLength:
		// Contagem de caracteres, não de bytes: é o que LENGTH devolve nos bancos com
		// charset multibyte, e acento em nome próprio é regra, não exceção.
		return int64(len([]rune(comoTexto(entradas[0])))), true
	case fnConcat:
		var b strings.Builder
		for _, entrada := range entradas {
			// Nulo como vazio, igual à compilação.
			if entrada == nil {
				continue
			}
			b.WriteString(comoTexto(entrada))
		}
		return b.String(), true
	case fnCoalesce:
		for _, entrada := range entradas {
			if entrada != nil {
				return entrada, true
			}
		}
		return nil, true
	case fnAno, fnMes, fnDia:
		instante, ok := comoInstante(entradas[0])
		if !ok {
			return nil, false
		}
		switch e.fn {
		case fnAno:
			return int64(instante.Year()), true
		case fnMes:
			return int64(instante.Month()), true
		default:
			return int64(instante.Day()), true
		}
	}
	return nil, false
}

// Evaluable diz se a expressão pode ser resolvida sem o banco. É falso para o
// EXISTS de relação, que precisa consultar a tabela filha.
func Evaluable(expressao Expression) bool {
	switch v := expressao.(type) {
	case Condition:
		_ = v
		return true
	case exprCondition:
		// As expressões do bloco 5 têm equivalente em Go, então a condição é
		// avaliável. A ressalva do Now() — relógio do Go em vez do banco — está no
		// próprio construtor.
		return true
	case Raw:
		// Fragmento cru precisa do banco. É o mesmo caso do EXISTS de relação.
		_ = v
		return false
	case Group:
		for _, it := range v.items {
			if it.expr != nil && !Evaluable(it.expr) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// valorDaLinha lê o campo da struct pelo Field.Name. Aceita struct, ponteiro para
// struct e Record.
func valorDaLinha(linha any, campo Field) (any, bool) {
	if registro, ok := linha.(Record); ok {
		if !registro.Has(campo) {
			return nil, false
		}
		return registro.Value(campo).Raw(), true
	}

	refletido := reflect.ValueOf(linha)
	for refletido.Kind() == reflect.Ptr || refletido.Kind() == reflect.Interface {
		if refletido.IsNil() {
			return nil, false
		}
		refletido = refletido.Elem()
	}
	if refletido.Kind() != reflect.Struct {
		return nil, false
	}
	valor := refletido.FieldByName(campo.Name)
	if !valor.IsValid() {
		return nil, false
	}
	// Coluna anulável vira ponteiro na struct gerada: nil é NULL de verdade.
	if valor.Kind() == reflect.Ptr {
		if valor.IsNil() {
			return nil, true
		}
		valor = valor.Elem()
	}
	if !valor.CanInterface() {
		return nil, false
	}
	return valor.Interface(), true
}

func avaliar(valor any, op Operator, esperados []any) bool {
	switch op {
	case IsNull:
		return valor == nil
	case IsNotNull:
		return valor != nil
	}
	if valor == nil {
		// NULL não satisfaz comparação em SQL, e aqui é igual — inclusive no
		// NotEqual, que é a parte que costuma surpreender.
		return false
	}

	switch op {
	case Equal:
		return len(esperados) > 0 && iguais(valor, esperados[0])
	case NotEqual:
		return len(esperados) > 0 && !iguais(valor, esperados[0])
	case GreaterThan, GreaterOrEqual, LessThan, LessOrEqual:
		if len(esperados) == 0 {
			return false
		}
		ordem, ok := comparar(valor, esperados[0])
		if !ok {
			return false
		}
		switch op {
		case GreaterThan:
			return ordem > 0
		case GreaterOrEqual:
			return ordem >= 0
		case LessThan:
			return ordem < 0
		default:
			return ordem <= 0
		}
	case Contains, StartsWith, EndsWith:
		// Sensível à caixa, de propósito: é o que o LIKE do SQL padrão faz, e é a
		// semântica do Postgres e do Oracle. MySQL e SQL Server são insensíveis por
		// COLLATION do banco, não por definição da cláusula — então alinhar aqui ao
		// LIKE deixa a divergência visível na bateria em vez de escondida atrás de
		// uma avaliação em memória mais permissiva que a consulta.
		texto := comoTexto(valor)
		if len(esperados) == 0 {
			return false
		}
		alvo := comoTexto(esperados[0])
		switch op {
		case Contains:
			return strings.Contains(texto, alvo)
		case StartsWith:
			return strings.HasPrefix(texto, alvo)
		default:
			return strings.HasSuffix(texto, alvo)
		}
	case In:
		for _, esperado := range esperados {
			if iguais(valor, esperado) {
				return true
			}
		}
		return false
	case Between:
		if len(esperados) < 2 {
			return false
		}
		inicio, ok1 := comparar(valor, esperados[0])
		fim, ok2 := comparar(valor, esperados[1])
		return ok1 && ok2 && inicio >= 0 && fim <= 0
	}
	return false
}

func iguais(a, b any) bool {
	if ordem, ok := comparar(a, b); ok {
		return ordem == 0
	}
	return false
}

// comparar devolve -1, 0 ou 1, e falso quando os dois lados não são comparáveis.
// Números de tipos diferentes (int64 da linha, int do literal) precisam comparar,
// então tudo numérico passa por float64.
func comparar(a, b any) (int, bool) {
	if instanteA, ok := comoInstante(a); ok {
		if instanteB, ok := comoInstante(b); ok {
			switch {
			case instanteA.Before(instanteB):
				return -1, true
			case instanteA.After(instanteB):
				return 1, true
			default:
				return 0, true
			}
		}
		return 0, false
	}
	if numeroA, ok := comoNumero(a); ok {
		if numeroB, ok := comoNumero(b); ok {
			switch {
			case numeroA < numeroB:
				return -1, true
			case numeroA > numeroB:
				return 1, true
			default:
				return 0, true
			}
		}
		return 0, false
	}
	if boolA, ok := a.(bool); ok {
		if boolB, ok := b.(bool); ok {
			if boolA == boolB {
				return 0, true
			}
			return 1, true
		}
		return 0, false
	}
	textoA, textoB := comoTexto(a), comoTexto(b)
	return strings.Compare(textoA, textoB), true
}

func comoNumero(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case Value:
		if n.IsNull() {
			return 0, false
		}
		return comoNumero(n.Raw())
	}
	return 0, false
}

func comoInstante(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	case Value:
		if t.IsNull() {
			return time.Time{}, false
		}
		return comoInstante(t.Raw())
	}
	return time.Time{}, false
}

func comoTexto(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case Value:
		return s.Text()
	case nil:
		return ""
	}
	return Value{bruto: v}.Text()
}
