package orm

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Value é o resultado de uma consulta escalar (agregação, Pluck). Uma agregação
// é dinâmica por natureza — MIN de uma data devolve data, de um id devolve
// número — então o motor lê o valor cru e o chamador extrai no tipo que espera.
// É o que corrige o Min/Max em coluna de data (antes tudo era lido como float64
// e a data estourava o Scan).
type Value struct{ bruto any }

// NewValue embala um valor cru (uso interno e de testes).
func NewValue(v any) Value { return Value{bruto: normalizeCell(v)} }

// MarshalJSON serializa o valor CRU, não o struct. Sem isto um Value devolvido
// numa resposta JSON sairia como "{}" (o campo interno é privado).
func (v Value) MarshalJSON() ([]byte, error) { return json.Marshal(v.bruto) }

// Nulo informa que a consulta não devolveu valor (ex.: agregação sem linhas).
func (v Value) IsNull() bool { return v.bruto == nil }

// Bruto devolve o valor como veio do driver.
func (v Value) Raw() any { return v.bruto }

func (v Value) String() string { return v.Text() }

// Texto converte para string (número e data viram texto legível).
func (v Value) Text() string {
	switch t := v.bruto.(type) {
	case nil:
		return ""
	case string:
		return t
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// Int converte para int64 (trunca fracionário). Zero quando nulo/inconvertível.
func (v Value) Int() int64 {
	switch t := v.bruto.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return int64(f)
		}
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

// Float converte para float64. Zero quando nulo/inconvertível.
func (v Value) Float() float64 {
	switch t := v.bruto.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int64:
		return float64(t)
	case int:
		return float64(t)
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f
		}
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

// Bool converte para booleano (0/1, "true"/"false").
func (v Value) Bool() bool {
	switch t := v.bruto.(type) {
	case bool:
		return t
	case int64:
		return t != 0
	case float64:
		return t != 0
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	}
	return false
}

// Tempo converte para time.Time, aceitando os formatos de data usuais em texto.
func (v Value) Time() time.Time {
	switch t := v.bruto.(type) {
	case time.Time:
		return t
	case string:
		if d, ok := parseDate(t); ok {
			return d
		}
	}
	return time.Time{}
}
