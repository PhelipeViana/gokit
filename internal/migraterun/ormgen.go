package migraterun

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/migration/acao"
)

// genRel descreve uma relação derivada do grafo de FK.
type genRel struct {
	name       string // nome do campo/relação
	kind       string // "belongsTo" | "hasMany"
	target     string // tabela de destino
	fkColumn   string // belongsTo: FK na própria; hasMany: FK no filho
	fkNullable bool
	jsonName   string // chave json (snake) do campo de relação
}

// GenerateORM rebuilds the application-owned mapping layer from migrations.
// Runtime behaviour never goes into this output: it remains in gokit/orm.
func GenerateORM(root string, state config.ConfigState) (int, error) {
	shapes, err := tableShapes(root, state)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(root, filepath.FromSlash(state.Config.Output.ORM), "fields.gen.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	names := make([]string, 0, len(shapes))
	for name := range shapes {
		names = append(names, name)
	}
	sort.Strings(names)

	// Deriva as relações do grafo de FK: belongsTo (FK na própria tabela) e
	// hasMany (inverso — toda FK que aponta para a tabela vira um hasMany nela).
	relsByTable := map[string][]genRel{}
	for _, name := range names {
		shape := shapes[name]
		for _, col := range shape.Columns {
			if col.ReferenceTable == "" {
				continue
			}
			relsByTable[shape.Table] = append(relsByTable[shape.Table], genRel{
				name: relNameFromFK(col.Name), kind: "belongsTo",
				target: col.ReferenceTable, fkColumn: col.Name, fkNullable: col.Nullable,
				jsonName: strings.TrimSuffix(strings.ToLower(col.Name), "_id"),
			})
		}
	}
	for _, name := range names {
		child := shapes[name]
		for _, col := range child.Columns {
			if col.ReferenceTable == "" {
				continue
			}
			relsByTable[col.ReferenceTable] = append(relsByTable[col.ReferenceTable], genRel{
				name: exportedORMIdentifier(child.Table), kind: "hasMany",
				target: child.Table, fkColumn: col.Name, fkNullable: col.Nullable,
				jsonName: strings.ToLower(child.Table),
			})
		}
	}
	// Desambigua nomes que colidem na mesma entidade: duas FKs para a MESMA
	// tabela (users ← pedidos.user_id e pedidos.aprovador_id, ambos "Pedidos"),
	// duas colunas que derivam o mesmo nome, ou relação com o mesmo nome de uma
	// coluna. Sem isso o arquivo gerado não compila ("Pedidos redeclared").
	for _, name := range names {
		tabela := shapes[name].Table
		colunas := map[string]bool{}
		for _, c := range shapes[name].Columns {
			colunas[exportedORMIdentifier(c.Name)] = true
		}
		relsByTable[tabela] = desambiguarRelacoes(relsByTable[tabela], colunas)
	}

	hasRelations := false
	for _, rs := range relsByTable {
		if len(rs) > 0 {
			hasRelations = true
			break
		}
	}

	var body strings.Builder
	var loaders strings.Builder
	var inits strings.Builder
	needsTime := false
	for _, name := range names {
		shape := shapes[name]
		entity := exportedORMIdentifier(shape.Table)       // ex.: Users
		unexported := unexportedORMIdentifier(shape.Table) // ex.: users

		// Linha tipada da entidade (retorno de Get/First). Coluna nula → ponteiro.
		fmt.Fprintf(&body, "type %sRow struct {\n", entity)
		for _, column := range shape.Columns {
			goType := rowGoType(column)
			if strings.Contains(goType, "time.Time") {
				needsTime = true
			}
			tag := "`json:\"" + column.Name + "\"`"
			fmt.Fprintf(&body, "\t%s %s %s\n", exportedORMIdentifier(column.Name), goType, tag)
		}
		// Campos de relação (preenchidos só quando pedidos via .With; omitempty
		// esconde os que não vieram na resposta JSON).
		for _, rel := range relsByTable[shape.Table] {
			targetRow := exportedORMIdentifier(rel.target) + "Row"
			tag := "`json:\"" + rel.jsonName + ",omitempty\"`"
			if rel.kind == "belongsTo" {
				fmt.Fprintf(&body, "\t%s *%s %s\n", rel.name, targetRow, tag)
			} else {
				fmt.Fprintf(&body, "\t%s []%s %s\n", rel.name, targetRow, tag)
			}
		}
		body.WriteString("}\n\n")

		// Conjunto de operadores por coluna, exposto em orm.Users.Field.<Coluna>.
		fmt.Fprintf(&body, "type %sFieldSet struct {\n", unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t%s %s\n", exportedORMIdentifier(column.Name), filterType(column.Type))
		}
		body.WriteString("}\n\n")

		// Conjunto de relações da entidade. Cada relação é um MÉTODO variádico que
		// recebe, em qualquer ordem, relações do destino (aninhamento) e colunas
		// do destino (projeção do retorno daquele nó):
		//
		//	pr.Cidade(cr.Estado(er.Field.Nome), cr.Field.Nome)
		//
		// O aninhamento por parênteses alcança profundidade ilimitada usando só as
		// relações próprias de cada entidade — por isso não geramos tipos por
		// caminho (que explodiriam em schema grande).
		fmt.Fprintf(&body, "type %sRelations struct {\n", unexported)
		for _, rel := range relsByTable[shape.Table] {
			fmt.Fprintf(&body, "\t%s gokitorm.Relation\n", campoPrivado(rel.name))
		}
		body.WriteString("}\n\n")
		for _, rel := range relsByTable[shape.Table] {
			fmt.Fprintf(&body, "func (r %sRelations) %s(args ...gokitorm.NodeArg) gokitorm.Relation {\n\treturn r.%s.Node(args...)\n}\n\n",
				unexported, rel.name, campoPrivado(rel.name))
		}

		// Handle único: embute o Model[Row] (promove Where/Select/OrderBy/Get/...
		// direto na entidade — orm.Users.Where(...)), + .Field (operadores) e
		// .Relation (relações).
		fmt.Fprintf(&body, "type %sEntity struct {\n\tgokitorm.Model[%sRow]\n\tField    %sFieldSet\n\tRelation %sRelations\n}\n\n", unexported, entity, unexported, unexported)

		// Scanner por NOME de coluna (robusto à ordem e à caixa que cada banco devolve).
		fmt.Fprintf(&body, "func scan%s(rows *sql.Rows) ([]%sRow, error) {\n", entity, entity)
		body.WriteString("\tcols, err := rows.Columns()\n\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		fmt.Fprintf(&body, "\tvar out []%sRow\n", entity)
		body.WriteString("\tfor rows.Next() {\n")
		fmt.Fprintf(&body, "\t\tvar row %sRow\n", entity)
		body.WriteString("\t\tdest := make([]any, len(cols))\n\t\tfor i, c := range cols {\n\t\t\tswitch strings.ToLower(c) {\n")
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t\t\tcase %q:\n\t\t\t\tdest[i] = &row.%s\n", strings.ToLower(column.Name), exportedORMIdentifier(column.Name))
		}
		body.WriteString("\t\t\tdefault:\n\t\t\t\tvar descarte any\n\t\t\t\tdest[i] = &descarte\n\t\t\t}\n\t\t}\n")
		body.WriteString("\t\tif err := rows.Scan(dest...); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tout = append(out, row)\n\t}\n\treturn out, rows.Err()\n}\n\n")

		// Construção num func literal para não repetir os literais de Field.
		fmt.Fprintf(&body, "var %s = func() %sEntity {\n", entity, unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t%s := gokitorm.Field{Name: %q, Entity: %q, Table: %q, Column: %q, DataType: %q, Nullable: %t, PrimaryKey: %t, AutoIncrement: %t, Unique: %t, Reference: %q}\n", localFieldVar(column.Name), exportedORMIdentifier(column.Name), shape.Table, shape.Table, column.Name, column.Type, column.Nullable, column.PrimaryKey, column.AutoIncrement, column.Unique, reference(column))
		}
		fmt.Fprintf(&body, "\treturn %sEntity{\n", unexported)
		fmt.Fprintf(&body, "\t\tModel: gokitorm.NewModel[%sRow](gokitorm.EntityFields{Name: %q, Fields: []gokitorm.Field{", entity, shape.Table)
		for i, column := range shape.Columns {
			if i > 0 {
				body.WriteString(", ")
			}
			body.WriteString(localFieldVar(column.Name))
		}
		fmt.Fprintf(&body, "}}, scan%s),\n", entity)
		fmt.Fprintf(&body, "\t\tField: %sFieldSet{\n", unexported)
		for _, column := range shape.Columns {
			fmt.Fprintf(&body, "\t\t\t%s: %s(%s),\n", exportedORMIdentifier(column.Name), filterConstructor(column.Type), localFieldVar(column.Name))
		}
		body.WriteString("\t\t},\n\t}\n}()\n\n")

		// O wiring das relações vai num init() (não no literal do var) para evitar
		// ciclo de inicialização entre vars de entidade (ex.: Cidades <-> Estados)
		// através dos loaders. Os loaders em si são funções, emitidas ao final.
		for _, rel := range relsByTable[shape.Table] {
			// correlação para EXISTS: belongsTo → FK no pai / id no filho; hasMany → id no pai / FK no filho.
			parentCol, childCol := "id", rel.fkColumn
			if rel.kind == "belongsTo" {
				parentCol, childCol = rel.fkColumn, "id"
			}
			fmt.Fprintf(&inits, "\t%s.Relation.%s = gokitorm.NewRelation(%q, %q, %q, %q, %q, load%s%s)\n",
				entity, campoPrivado(rel.name), rel.name, rel.kind, rel.target, parentCol, childCol, entity, rel.name)
			emitLoader(&loaders, entity, rel.name, rel.kind, rel.target, rel.fkColumn, rel.fkNullable)
		}
	}
	if hasRelations {
		body.WriteString("func init() {\n")
		body.WriteString(inits.String())
		body.WriteString("}\n\n")
	}
	body.WriteString(loaders.String())

	var out strings.Builder
	out.WriteString("// Code generated by GoKit from migrations. DO NOT EDIT.\npackage orm\n\n")
	if len(names) > 0 {
		out.WriteString("import (\n\t\"database/sql\"\n")
		if hasRelations {
			out.WriteString("\t\"context\"\n\t\"fmt\"\n")
		}
		out.WriteString("\t\"strings\"\n")
		if needsTime {
			out.WriteString("\t\"time\"\n")
		}
		out.WriteString("\n\tgokitorm \"github.com/PhelipeViana/gokit/orm\"\n)\n\n")
	}
	out.WriteString(body.String())
	formatted, err := format.Source([]byte(out.String()))
	if err != nil {
		return 0, err
	}
	_ = os.Remove(target)
	if err := os.WriteFile(target, formatted, 0o644); err != nil {
		return 0, err
	}

	// response.gen.go — envelope padrão de resposta JSON (data/meta/error).
	if err := writeResponseEnvelope(filepath.Dir(target)); err != nil {
		return len(names), err
	}
	return len(names), nil
}

// writeResponseEnvelope gera o response.gen.go: porta fiel do padrão do cliente
// (internal/shared/response — payloads success/message/data|errors/meta, tabela
// de status pt-BR, paginação e Central/Recorder) + a ponte genérica com a ORM
// (RespondLista/RespondItem/RespondDados). O único acoplamento do cliente
// (requestctx, fuso) vira plugável (RequestIDFn, Local). Template com § no lugar
// de crase (evita escapar aspas).
func writeResponseEnvelope(dir string) error {
	tpl := `// Code generated by GoKit. DO NOT EDIT.
package orm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	gokitorm "github.com/PhelipeViana/gokit/orm"
)

// ── Ajustes plugáveis (a aplicação seta no boot, se quiser) ──

// RequestIDFn extrai o request_id do contexto (ex.: um middleware). Default: vazio.
var RequestIDFn = func(context.Context) string { return "" }

// Local é o fuso usado no timestamp de meta. Default: fuso local do servidor.
var Local = time.Local

const StatusOK = "Operação realizada com sucesso"

var statusTexts = map[int]string{
	http.StatusOK:        StatusOK,
	http.StatusCreated:   "Criado com sucesso",
	http.StatusAccepted:  "Aceito",
	http.StatusNoContent: "Sem conteúdo",

	http.StatusBadRequest:            "Requisição inválida",
	http.StatusUnauthorized:          "Não autorizado: sessão inválida ou ausente",
	http.StatusForbidden:             "Acesso proibido",
	http.StatusNotFound:              "Recurso não encontrado",
	http.StatusMethodNotAllowed:      "Método não permitido",
	http.StatusConflict:              "Conflito",
	http.StatusUnprocessableEntity:   "Entidade não processável",
	http.StatusTooManyRequests:       "Muitas requisições",

	http.StatusInternalServerError: "Erro interno do servidor.",
	http.StatusNotImplemented:      "Não implementado",
	http.StatusBadGateway:          "Gateway inválido",
	http.StatusServiceUnavailable:  "Serviço indisponível",
	http.StatusGatewayTimeout:      "Tempo limite do gateway esgotado",
}

// DefaultStatusText devolve a mensagem padrão em português para um status HTTP.
func DefaultStatusText(status int) string {
	if text, ok := statusTexts[status]; ok {
		return text
	}
	return http.StatusText(status)
}

type Meta struct {
	Timestamp  string          §json:"timestamp"§
	RequestID  string          §json:"request_id"§
	DurationMs *int64          §json:"duration_ms,omitempty"§
	Pagination *PaginationInfo §json:"pagination,omitempty"§
}

type PaginationInfo struct {
	Page     int   §json:"page"§
	PerPage  int   §json:"per_page"§
	Total    int64 §json:"total"§
	LastPage int   §json:"last_page"§
}

type SuccessPayload struct {
	Success bool   §json:"success"§
	Message string §json:"message"§
	Data    any    §json:"data"§
	Meta    Meta   §json:"meta"§
}

type ErrorPayload struct {
	Success bool   §json:"success"§
	Message string §json:"message"§
	Errors  any    §json:"errors"§
	Meta    Meta   §json:"meta"§
}

// Success responde com sucesso (payload padrão) no status informado.
func Success(w http.ResponseWriter, r *http.Request, status int, data any, message ...string) {
	emit(w, r, status, SuccessPayload{Success: true, Message: mensagemOuPadrao(status, message), Data: data, Meta: newMeta(r, nil)})
}

// SuccessPaginated responde com sucesso incluindo os metadados de paginação em meta.
func SuccessPaginated(w http.ResponseWriter, r *http.Request, status int, data any, pagination PaginationInfo, message ...string) {
	emit(w, r, status, SuccessPayload{Success: true, Message: mensagemOuPadrao(status, message), Data: data, Meta: newMeta(r, &pagination)})
}

// Error responde um erro no status informado, aceitando string ou error como mensagem.
func Error(w http.ResponseWriter, r *http.Request, status int, args ...any) {
	message := DefaultStatusText(status)
	if len(args) > 0 {
		switch v := args[0].(type) {
		case string:
			if v != "" {
				message = v
			}
		case error:
			if v != nil {
				message = v.Error()
			}
		}
	}
	emitError(w, r, status, message, nil)
}

// ErrorWithDetails responde um erro com mensagem e detalhes por campo.
func ErrorWithDetails(w http.ResponseWriter, r *http.Request, status int, message string, details any) {
	emitError(w, r, status, message, details)
}

// emit entrega o payload à Central de Resposta quando ela está no contexto; sem
// Central, serializa direto.
func emit(w http.ResponseWriter, r *http.Request, status int, payload any) {
	if rec, ok := RecorderFrom(r.Context()); ok && rec.Record(status, payload) {
		return
	}
	WriteJSON(w, status, payload)
}

// emitError aplica o mascaramento de 5xx (safeError) só quando NÃO há Central.
func emitError(w http.ResponseWriter, r *http.Request, status int, message string, details any) {
	if rec, ok := RecorderFrom(r.Context()); ok {
		if rec.Record(status, ErrorPayload{Success: false, Message: message, Errors: details, Meta: newMeta(r, nil)}) {
			return
		}
	}
	message, details = safeError(r, status, message, details)
	WriteJSON(w, status, ErrorPayload{Success: false, Message: message, Errors: details, Meta: newMeta(r, nil)})
}

func mensagemOuPadrao(status int, message []string) string {
	if len(message) > 0 && message[0] != "" {
		return message[0]
	}
	return DefaultStatusText(status)
}

func newMeta(r *http.Request, pagination *PaginationInfo) Meta {
	return Meta{Timestamp: now(), RequestID: RequestIDFn(r.Context()), Pagination: pagination}
}

// safeError mascara falhas 5xx: loga a causa real com o request_id e devolve mensagem genérica.
func safeError(r *http.Request, status int, message string, details any) (string, any) {
	if status < http.StatusInternalServerError {
		return message, details
	}
	log.Printf("erro interno request_id=%s: %s", RequestIDFn(r.Context()), message)
	return "Erro interno do servidor.", nil
}

// WriteJSON serializa o payload como JSON e escreve a resposta com o status informado.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func now() string { return time.Now().In(Local).Format(time.RFC3339) }

// ── Central de Resposta (Recorder): hook opcional (a plataforma HTTP implementa) ──

type Recorder interface {
	Record(status int, payload any) bool
}

type recorderKey struct{}

func WithRecorder(ctx context.Context, rec Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, rec)
}

func RecorderFrom(ctx context.Context) (Recorder, bool) {
	rec, ok := ctx.Value(recorderKey{}).(Recorder)
	return rec, ok && rec != nil
}

// ── Paginação (parse + validação, iguais ao padrão do cliente) ──

type PaginationParams struct {
	Page    int
	PerPage int
	Offset  int
}

const (
	DefaultPage    = 1
	DefaultPerPage = 15
	MaxPerPage     = 100
)

// ParsePagination lê page/per_page da query e valida, com erros por campo.
func ParsePagination(r *http.Request) (PaginationParams, map[string][]string) {
	errs := map[string][]string{}
	page := DefaultPage
	if s := r.URL.Query().Get("page"); s != "" {
		if p, err := strconv.Atoi(s); err != nil {
			errs["page"] = append(errs["page"], "Campo deve ser inteiro válido")
		} else if p <= 0 {
			errs["page"] = append(errs["page"], "Valor deve ser maior que zero")
		} else {
			page = p
		}
	}
	perPage := DefaultPerPage
	if s := r.URL.Query().Get("per_page"); s != "" {
		if l, err := strconv.Atoi(s); err != nil {
			errs["per_page"] = append(errs["per_page"], "Campo deve ser inteiro válido")
		} else if l <= 0 {
			errs["per_page"] = append(errs["per_page"], "Valor deve ser maior que zero")
		} else if l > MaxPerPage {
			errs["per_page"] = append(errs["per_page"], "Valor deve ser menor ou igual a 100")
		} else {
			perPage = l
		}
	}
	return PaginationParams{Page: page, PerPage: perPage, Offset: (page - 1) * perPage}, errs
}

// NewPaginationInfo monta os metadados de paginação a partir do total e dos parâmetros.
func NewPaginationInfo(total int64, params PaginationParams) PaginationInfo {
	lastPage := int((total + int64(params.PerPage) - 1) / int64(params.PerPage))
	if lastPage < 1 {
		lastPage = 1
	}
	return PaginationInfo{Page: params.Page, PerPage: params.PerPage, Total: total, LastPage: lastPage}
}

// ── Ponte genérica com a ORM: o handler só monta a query e chama um Respond* ──

// StatusError carrega um status HTTP + mensagem; o responder deriva o código dele.
type StatusError struct {
	Status  int
	Message string
}

func (e StatusError) Error() string { return e.Message }

func ErroRequisicao(msg string) error    { return StatusError{Status: http.StatusBadRequest, Message: msg} }
func ErroNaoEncontrado(msg string) error { return StatusError{Status: http.StatusNotFound, Message: msg} }
func ErroConflito(msg string) error      { return StatusError{Status: http.StatusConflict, Message: msg} }

func statusDe(err error) int {
	var se StatusError
	if errors.As(err, &se) && se.Status != 0 {
		return se.Status
	}
	// ErrNaoEncontrado (First/Find sem linha) vira 404 sem o handler precisar
	// testar nada — a informação já vem no erro.
	if errors.Is(err, gokitorm.ErrNaoEncontrado) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// RespondLista valida page/per_page, pagina a query e responde paginado.
func RespondLista[T any](w http.ResponseWriter, r *http.Request, q gokitorm.Query[T]) {
	params, errs := ParsePagination(r)
	if len(errs) > 0 {
		ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "Parâmetros de paginação inválidos", errs)
		return
	}
	res, err := q.Paginate(r.Context(), params.Page, params.PerPage)
	if err != nil {
		Error(w, r, statusDe(err), err)
		return
	}
	SuccessPaginated(w, r, http.StatusOK, res.Items, NewPaginationInfo(res.Total, params))
}

// RespondItem responde {data}; o 404 sai do próprio erro (ErrNaoEncontrado), e
// a mensagem de 5xx é mascarada — o handler só repassa (valor, erro).
func RespondItem[T any](w http.ResponseWriter, r *http.Request, v T, err error) {
	if err != nil {
		if errors.Is(err, gokitorm.ErrNaoEncontrado) {
			Error(w, r, http.StatusNotFound) // usa a mensagem padrão do status
			return
		}
		Error(w, r, statusDe(err), err)
		return
	}
	Success(w, r, http.StatusOK, v)
}

// RespondDados responde {data} para um payload arbitrário (stats, agregações...).
func RespondDados(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err != nil {
		Error(w, r, statusDe(err), err)
		return
	}
	Success(w, r, http.StatusOK, v)
}

// RespondErro responde um erro com o status derivado dele.
func RespondErro(w http.ResponseWriter, r *http.Request, err error) { Error(w, r, statusDe(err), err) }
`
	src := strings.ReplaceAll(tpl, "§", "`")
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "response.gen.go")
	_ = os.Remove(target)
	return os.WriteFile(target, formatted, 0o644)
}

func reference(column acao.ColunaDefinicao) string {
	if column.ReferenceTable == "" {
		return ""
	}
	return column.ReferenceTable + "." + column.ReferenceColumn
}

// rowGoType mapeia o tipo SQL da coluna para o tipo Go da linha tipada.
// Coluna anulável vira ponteiro para representar NULL.
func rowGoType(column acao.ColunaDefinicao) string {
	base := "string"
	switch strings.ToLower(column.Type) {
	case "integer", "int":
		base = "int64"
	case "decimal":
		base = "float64"
	case "boolean":
		base = "bool"
	case "date", "datetime", "timestamp":
		base = "time.Time"
	}
	if column.Nullable {
		return "*" + base
	}
	return base
}

func filterType(kind string) string {
	switch strings.ToLower(kind) {
	case "integer", "int", "decimal":
		return "gokitorm.NumberFilterField"
	case "boolean":
		return "gokitorm.BoolFilterField"
	case "date", "datetime", "timestamp":
		return "gokitorm.DateFilterField"
	default:
		return "gokitorm.StringFilterField"
	}
}
func filterConstructor(kind string) string {
	switch filterType(kind) {
	case "gokitorm.NumberFilterField":
		return "gokitorm.NumberFilter"
	case "gokitorm.BoolFilterField":
		return "gokitorm.BoolFilter"
	case "gokitorm.DateFilterField":
		return "gokitorm.DateFilter"
	default:
		return "gokitorm.StringFilter"
	}
}
func exportedORMIdentifier(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for i := range parts {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// desambiguarRelacoes garante nomes únicos entre as relações de uma entidade (e
// distintos das colunas). Quem colide passa a carregar a FK no nome:
//
//	hasMany   users ← pedidos.aprovador_id  →  PedidosPorAprovador (pedidos_por_aprovador)
//	belongsTo colisão de coluna             →  <Coluna>Rel        (<coluna>_rel)
//
// A ordem de entrada é estável (colunas e tabelas percorridas ordenadas), então o
// resultado é determinístico entre execuções.
func desambiguarRelacoes(rels []genRel, colunas map[string]bool) []genRel {
	if len(rels) == 0 {
		return rels
	}
	ocorrencias := map[string]int{}
	for _, r := range rels {
		ocorrencias[r.name]++
	}

	usados := map[string]bool{}
	out := make([]genRel, 0, len(rels))
	for _, r := range rels {
		if ocorrencias[r.name] > 1 || colunas[r.name] {
			r.name, r.jsonName = nomeDesambiguado(r)
		}
		// Garantia final: nunca emitir dois campos com o mesmo nome.
		for usados[r.name] || colunas[r.name] {
			r.name += "Rel"
			r.jsonName += "_rel"
		}
		usados[r.name] = true
		out = append(out, r)
	}
	return out
}

// nomeDesambiguado devolve nome Go e chave json de uma relação em conflito.
func nomeDesambiguado(r genRel) (string, string) {
	if r.kind == "hasMany" {
		viaFK := strings.TrimSuffix(strings.ToLower(r.fkColumn), "_id")
		return exportedORMIdentifier(r.target) + "Por" + exportedORMIdentifier(viaFK),
			strings.ToLower(r.target) + "_por_" + viaFK
	}
	return exportedORMIdentifier(r.fkColumn) + "Rel", strings.ToLower(r.fkColumn) + "_rel"
}

// campoPrivado devolve o nome do campo interno que guarda a Relation base:
// PedidosPorUser -> pedidosPorUser (só a 1ª letra muda; preserva o camelCase).
func campoPrivado(nome string) string {
	if nome == "" {
		return ""
	}
	return strings.ToLower(nome[:1]) + nome[1:]
}

// relNameFromFK deriva o nome da relação belongsTo a partir da coluna FK:
// cidade_id -> Cidade, pais_id -> Pais. Sem sufixo _id, usa a coluna inteira.
func relNameFromFK(col string) string {
	base := col
	if strings.HasSuffix(strings.ToLower(col), "_id") {
		base = col[:len(col)-3]
	}
	return exportedORMIdentifier(base)
}

// emitLoader escreve o loader tipado de uma relação (belongsTo ou hasMany) que
// o descritor gokitorm.Relation carrega. Faz o type-assert dos pais e delega
// para os helpers genéricos do motor.
func emitLoader(b *strings.Builder, selfEntity, relName, kind, target, fkColumn string, fkNullable bool) {
	selfRow := selfEntity + "Row"
	targetEntity := exportedORMIdentifier(target)
	targetRow := targetEntity + "Row"
	loaderName := "load" + selfEntity + relName

	fmt.Fprintf(b, "func %s(ctx context.Context, r gokitorm.Runner, parentsAny any, rel gokitorm.Relation) error {\n", loaderName)
	fmt.Fprintf(b, "\tparents, ok := parentsAny.([]%s)\n", selfRow)
	fmt.Fprintf(b, "\tif !ok {\n\t\treturn fmt.Errorf(\"relação %s: pais não são []%s\")\n\t}\n", relName, selfRow)

	if kind == "belongsTo" {
		fkField := exportedORMIdentifier(fkColumn)
		b.WriteString("\treturn gokitorm.BelongsTo(ctx, r, parents,\n")
		if fkNullable {
			fmt.Fprintf(b, "\t\tfunc(p %s) (int64, bool) {\n\t\t\tif p.%s == nil {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn *p.%s, true\n\t\t},\n", selfRow, fkField, fkField)
		} else {
			fmt.Fprintf(b, "\t\tfunc(p %s) (int64, bool) { return p.%s, true },\n", selfRow, fkField)
		}
		fmt.Fprintf(b, "\t\t%s.Model, %s.Field.Id,\n", targetEntity, targetEntity)
		fmt.Fprintf(b, "\t\tfunc(c %s) int64 { return c.Id },\n", targetRow)
		fmt.Fprintf(b, "\t\tfunc(p *%s, c *%s) { p.%s = c },\n", selfRow, targetRow, relName)
		b.WriteString("\t\trel)\n}\n\n")
		return
	}

	childFk := exportedORMIdentifier(fkColumn)
	b.WriteString("\treturn gokitorm.HasMany(ctx, r, parents,\n")
	fmt.Fprintf(b, "\t\tfunc(p %s) int64 { return p.Id },\n", selfRow)
	fmt.Fprintf(b, "\t\t%s.Model, %s.Field.%s,\n", targetEntity, targetEntity, childFk)
	if fkNullable {
		fmt.Fprintf(b, "\t\tfunc(c %s) (int64, bool) {\n\t\t\tif c.%s == nil {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn *c.%s, true\n\t\t},\n", targetRow, childFk, childFk)
	} else {
		fmt.Fprintf(b, "\t\tfunc(c %s) (int64, bool) { return c.%s, true },\n", targetRow, childFk)
	}
	fmt.Fprintf(b, "\t\tfunc(p *%s, cs []%s) { p.%s = cs },\n", selfRow, targetRow, relName)
	b.WriteString("\t\trel)\n}\n\n")
}

// unexportedORMIdentifier devolve o identificador em camelCase não-exportado,
// usado só como prefixo de tipos gerados (usersEntity, usersFieldSet).
func unexportedORMIdentifier(value string) string {
	e := exportedORMIdentifier(value)
	if e == "" {
		return ""
	}
	return strings.ToLower(e[:1]) + e[1:]
}

// localFieldVar nomeia a variável local do Field dentro do func literal gerado.
// O prefixo "col" garante identificador válido mesmo para colunas com nome de
// palavra reservada (type, func, range...).
func localFieldVar(column string) string {
	return "col" + exportedORMIdentifier(column)
}
