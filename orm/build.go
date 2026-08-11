package orm

import (
	"strconv"
	"strings"
	"time"
)

// When inclui expr no filtro apenas quando cond é verdadeiro; caso contrário
// devolve um grupo vazio, que o compilador ignora. Serve para condições
// arbitrárias (flags, switches) sem espalhar if pelo handler:
//
//	q.Where(When(temPedido, u.Relation.Pedidos.Exists()))
func When(cond bool, expr Expression) Expression {
	if !cond {
		return Group{}
	}
	return expr
}

// Filter monta um filtro a partir de um valor CRU (ex.: uma query string). É o
// atalho para pesquisa dinâmica: se raw vier vazio, o filtro é IGNORADO (não
// entra no WHERE) — acaba o "if campo-a-campo". O valor é convertido para o tipo
// da coluna (Field.DataType); conversão inválida também é ignorada. Ops de texto
// (Contains/StartsWith/EndsWith) usam raw como string; In aceita lista separada
// por vírgula.
//
//	q.Where(
//	    orm.Filter(f.Nome,     orm.Contains, qp.Get("nome")),
//	    orm.Filter(f.CidadeId, orm.Equal,    qp.Get("cidade_id")),
//	)
func Filter(col Column, op Operator, raw string) Expression {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Group{}
	}
	f := col.columnField()

	switch op {
	case Contains, StartsWith, EndsWith:
		return condition(f, op, raw) // LIKE opera sobre texto cru

	case In:
		var vals []any
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			v, ok := coerceValue(f.DataType, p)
			if !ok {
				return Group{}
			}
			vals = append(vals, v)
		}
		if len(vals) == 0 {
			return Group{}
		}
		return condition(f, In, vals...)

	default:
		v, ok := coerceValue(f.DataType, raw)
		if !ok {
			return Group{}
		}
		return condition(f, op, v)
	}
}

// coerceValue converte a string crua para o tipo Go da coluna. ok=false quando
// a conversão falha (o Filter então ignora a condição).
func coerceValue(dataType, raw string) (any, bool) {
	switch strings.ToLower(dataType) {
	case "integer", "int":
		n, err := strconv.ParseInt(raw, 10, 64)
		return n, err == nil
	case "decimal":
		v, err := strconv.ParseFloat(raw, 64)
		return v, err == nil
	case "boolean":
		b, err := strconv.ParseBool(raw)
		return b, err == nil
	case "date", "datetime", "timestamp":
		if t, ok := parseDate(raw); ok {
			return t, true
		}
		return nil, false
	default:
		return raw, true // texto
	}
}

// dateLayouts são os layouts aceitos num valor cru de data (query string, form).
// Convertemos para time.Time em vez de repassar a string: assim o driver manda o
// tipo certo e a pesquisa não depende de conversão implícita do banco.
var dateLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

func parseDate(raw string) (time.Time, bool) {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// DateValue é o tratamento genérico de data usado por TODOS os filtros de data:
// converte para time.Time o que for convertível (string em formato conhecido,
// *time.Time, Value) e devolve o resto intacto. Assim o driver recebe o tipo
// certo e a pesquisa não depende de conversão implícita do banco — que é onde
// Oracle e SQL Server divergem do MySQL.
func DateValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		return t
	case *time.Time:
		if t == nil {
			return nil
		}
		return *t
	case Value:
		return DateValue(t.Raw())
	case string:
		if d, ok := parseDate(t); ok {
			return d
		}
		return t // deixa passar: pode ser expressão que o chamador quis mesmo
	default:
		return v
	}
}

// dateValues aplica DateValue a uma lista (In, Between).
func dateValues(vs []any) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = DateValue(v)
	}
	return out
}
