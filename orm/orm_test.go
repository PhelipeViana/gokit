package orm_test

import (
	"reflect"
	"testing"
	"time"

	orm "github.com/PhelipeViana/gokit/orm"
)

func testModel() (orm.Model[orm.Record], struct {
	Id    orm.NumberFilterField
	Nome  orm.StringFilterField
	Idade orm.NumberFilterField
	Ativo orm.BoolFilterField
}) {
	id := orm.Field{Name: "Id", Entity: "users", Table: "users", Column: "id", DataType: "integer", PrimaryKey: true}
	nome := orm.Field{Name: "Nome", Entity: "users", Table: "users", Column: "nome", DataType: "string"}
	idade := orm.Field{Name: "Idade", Entity: "users", Table: "users", Column: "idade", DataType: "integer"}
	ativo := orm.Field{Name: "Ativo", Entity: "users", Table: "users", Column: "ativo", DataType: "boolean"}
	model := orm.NewModel[orm.Record](orm.EntityFields{Name: "users", Fields: []orm.Field{id, nome, idade, ativo}}, orm.ScanRecords)
	f := struct {
		Id    orm.NumberFilterField
		Nome  orm.StringFilterField
		Idade orm.NumberFilterField
		Ativo orm.BoolFilterField
	}{
		Id:    orm.NumberFilter(id),
		Nome:  orm.StringFilter(nome),
		Idade: orm.NumberFilter(idade),
		Ativo: orm.BoolFilter(ativo),
	}
	return model, f
}

func mustCompile(t *testing.T, q orm.Query[orm.Record], op orm.Operation) orm.CompiledQuery {
	t.Helper()
	c, err := q.Compile(op, orm.CompileOptions{Dialect: orm.Oracle})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func TestCompileEqualParametriza(t *testing.T) {
	m, f := testModel()
	c := mustCompile(t, m.Where(f.Nome.Equal("silva")), orm.Select)
	want := `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" WHERE "NOME" = :1`
	if c.SQL != want {
		t.Fatalf("SQL:\n got %q\nwant %q", c.SQL, want)
	}
	if !reflect.DeepEqual(c.Args, []any{"silva"}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestCompileComparadoresEBetweenEIn(t *testing.T) {
	m, f := testModel()
	q := m.Where(
		f.Idade.GreaterOrEqual(18),
		f.Idade.LessThan(65),
		f.Id.In(1, 2, 3),
		f.Idade.Between(20, 30),
	)
	c := mustCompile(t, q, orm.Count)
	want := `SELECT COUNT(*) FROM "USERS" WHERE "IDADE" >= :1 AND "IDADE" < :2 AND "ID" IN (:3, :4, :5) AND "IDADE" BETWEEN :6 AND :7`
	if c.SQL != want {
		t.Fatalf("SQL:\n got %q\nwant %q", c.SQL, want)
	}
	if !reflect.DeepEqual(c.Args, []any{18, 65, 1, 2, 3, 20, 30}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestCompileLikeVariantes(t *testing.T) {
	m, f := testModel()
	c := mustCompile(t, m.Where(f.Nome.Contains("a"), f.Nome.StartsWith("b"), f.Nome.EndsWith("c")), orm.Select)
	if want := `WHERE "NOME" LIKE :1 AND "NOME" LIKE :2 AND "NOME" LIKE :3`; !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q:\n%s", want, c.SQL)
	}
	if !reflect.DeepEqual(c.Args, []any{"%a%", "b%", "%c"}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestCombinadorOrDentroDeWhere(t *testing.T) {
	m, f := testModel()
	// Where(a, Or(b, c)) => a AND (b OR c), com parênteses explícitos.
	q := m.Where(f.Nome.Equal("silva"), orm.Or(f.Id.Equal(1), f.Ativo.IsTrue()))
	c := mustCompile(t, q, orm.Select)
	want := `WHERE "NOME" = :1 AND ("ID" = :2 OR "ATIVO" = :3)`
	if !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q:\n%s", want, c.SQL)
	}
	if !reflect.DeepEqual(c.Args, []any{"silva", 1, true}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestCombinadoresAninhados(t *testing.T) {
	m, f := testModel()
	// And(a, Or(b, c)) aninhado dentro de Where.
	q := m.Where(orm.And(f.Ativo.IsTrue(), orm.Or(f.Idade.LessThan(18), f.Idade.GreaterThan(65))))
	c := mustCompile(t, q, orm.Select)
	want := `WHERE ("ATIVO" = :1 AND ("IDADE" < :2 OR "IDADE" > :3))`
	if !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q:\n%s", want, c.SQL)
	}
}

func TestFiltroInativoEhIgnorado(t *testing.T) {
	m, f := testModel()
	// In() sem valores => inativo => não gera WHERE.
	c := mustCompile(t, m.Where(f.Nome.In()), orm.Select)
	if contains(c.SQL, "WHERE") {
		t.Fatalf("filtro vazio não deveria gerar WHERE: %s", c.SQL)
	}
	if len(c.Args) != 0 {
		t.Fatalf("Args deveria estar vazio: %#v", c.Args)
	}
}

func TestNulosEOrdenacaoEPaginacao(t *testing.T) {
	m, f := testModel()
	q := m.Where(f.Nome.IsNotNull()).OrderByDesc(f.Id).OrderBy(f.Nome).Offset(40).Limit(20)
	c := mustCompile(t, q, orm.Select)
	want := `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" WHERE "NOME" IS NOT NULL ORDER BY "ID" DESC, "NOME" ASC OFFSET 40 ROWS FETCH NEXT 20 ROWS ONLY`
	if c.SQL != want {
		t.Fatalf("SQL:\n got %q\nwant %q", c.SQL, want)
	}
}

func TestFiltroDinamico(t *testing.T) {
	m, f := testModel()
	q := m.Where(
		orm.Filter(f.Nome, orm.Contains, ""),       // vazio → ignorado
		orm.Filter(f.Nome, orm.Contains, "silva"),  // LIKE (texto)
		orm.Filter(f.Idade, orm.GreaterThan, "18"), // integer convertido
		orm.Filter(f.Id, orm.In, "1, 2 ,3"),        // IN por vírgula (com espaços)
		orm.Filter(f.Idade, orm.Equal, "abc"),      // parse inválido → ignorado
	)
	c := mustCompile(t, q, orm.Select)
	want := `WHERE "NOME" LIKE :1 AND "IDADE" > :2 AND "ID" IN (:3, :4, :5)`
	if !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q:\n%s", want, c.SQL)
	}
	if !reflect.DeepEqual(c.Args, []any{"%silva%", int64(18), int64(1), int64(2), int64(3)}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestWhenCondicional(t *testing.T) {
	m, f := testModel()
	c := mustCompile(t, m.Where(
		f.Ativo.IsTrue(),
		orm.When(false, f.Id.Equal(9)),    // ignorado
		orm.When(true, f.Nome.Equal("x")), // incluído
	), orm.Select)
	want := `WHERE "ATIVO" = :1 AND "NOME" = :2`
	if !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q:\n%s", want, c.SQL)
	}
	if !reflect.DeepEqual(c.Args, []any{true, "x"}) {
		t.Fatalf("Args: %#v", c.Args)
	}
}

func TestSelectProjetaColunas(t *testing.T) {
	m, f := testModel()
	c := mustCompile(t, m.Select(f.Id, f.Nome).Where(f.Ativo.IsTrue()), orm.Select)
	want := `SELECT "ID", "NOME" FROM "USERS" WHERE "ATIVO" = :1`
	if c.SQL != want {
		t.Fatalf("SQL:\n got %q\nwant %q", c.SQL, want)
	}
	// Sem Select => todas as colunas.
	full := mustCompile(t, m.Where(f.Ativo.IsTrue()), orm.Select)
	if !contains(full.SQL, `"ID", "NOME", "IDADE", "ATIVO"`) {
		t.Fatalf("sem Select deveria trazer todas as colunas: %s", full.SQL)
	}
}

func TestExistsFetchFirst(t *testing.T) {
	m, f := testModel()
	c := mustCompile(t, m.Where(f.Id.Equal(7)), orm.Exists)
	want := `SELECT 1 FROM "USERS" WHERE "ID" = :1 FETCH FIRST 1 ROWS ONLY`
	if c.SQL != want {
		t.Fatalf("SQL:\n got %q\nwant %q", c.SQL, want)
	}
}

func TestImutabilidadeDaQueryBase(t *testing.T) {
	m, f := testModel()
	base := m.Where(f.Ativo.IsTrue())
	_ = base.Where(f.Idade.GreaterThan(18)).Limit(5)
	// base não pode ter sido afetada pelo encadeamento acima.
	c := mustCompile(t, base, orm.Select)
	want := `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" WHERE "ATIVO" = :1`
	if c.SQL != want {
		t.Fatalf("query base foi mutada:\n got %q\nwant %q", c.SQL, want)
	}
}

func TestDialetoNaoSuportado(t *testing.T) {
	m, f := testModel()
	_, err := m.Where(f.Id.Equal(1)).Compile(orm.Select, orm.CompileOptions{Dialect: orm.Dialect("sqlite")})
	if err == nil {
		t.Fatal("esperava erro para dialeto não suportado")
	}
}

// TestPlaceholderEQuotePorDialeto fixa o marcador e o quoting de cada banco
// para a MESMA pesquisa, cobrindo a divergência que já mordeu no motor.
func TestPlaceholderEQuotePorDialeto(t *testing.T) {
	m, f := testModel()
	q := m.Where(f.Nome.Equal("silva"), f.Idade.GreaterThan(18))
	casos := map[orm.Dialect]string{
		orm.Oracle:    `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" WHERE "NOME" = :1 AND "IDADE" > :2`,
		orm.Postgres:  `SELECT "id", "nome", "idade", "ativo" FROM "users" WHERE "nome" = $1 AND "idade" > $2`,
		orm.MySQL:     "SELECT `id`, `nome`, `idade`, `ativo` FROM `users` WHERE `nome` = ? AND `idade` > ?",
		orm.SQLServer: `SELECT [id], [nome], [idade], [ativo] FROM [users] WHERE [nome] = @p1 AND [idade] > @p2`,
	}
	for dialect, want := range casos {
		c, err := q.Compile(orm.Select, orm.CompileOptions{Dialect: dialect})
		if err != nil {
			t.Fatalf("%s: %v", dialect, err)
		}
		if c.SQL != want {
			t.Fatalf("%s SQL:\n got %q\nwant %q", dialect, c.SQL, want)
		}
		if !reflect.DeepEqual(c.Args, []any{"silva", 18}) {
			t.Fatalf("%s Args: %#v", dialect, c.Args)
		}
	}
}

// TestPaginacaoPorDialeto cobre limit+offset, que é o ponto onde a sintaxe mais
// diverge (Oracle/SQL Server OFFSET-FETCH vs Postgres/MySQL LIMIT-OFFSET, e o
// TOP do SQL Server quando não há offset).
func TestPaginacaoPorDialeto(t *testing.T) {
	m, f := testModel()

	comOffset := m.Where(f.Ativo.IsTrue()).OrderByDesc(f.Id).Offset(40).Limit(20)
	wantOffset := map[orm.Dialect]string{
		orm.Oracle:    `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" WHERE "ATIVO" = :1 ORDER BY "ID" DESC OFFSET 40 ROWS FETCH NEXT 20 ROWS ONLY`,
		orm.Postgres:  `SELECT "id", "nome", "idade", "ativo" FROM "users" WHERE "ativo" = $1 ORDER BY "id" DESC LIMIT 20 OFFSET 40`,
		orm.MySQL:     "SELECT `id`, `nome`, `idade`, `ativo` FROM `users` WHERE `ativo` = ? ORDER BY `id` DESC LIMIT 20 OFFSET 40",
		orm.SQLServer: `SELECT [id], [nome], [idade], [ativo] FROM [users] WHERE [ativo] = @p1 ORDER BY [id] DESC OFFSET 40 ROWS FETCH NEXT 20 ROWS ONLY`,
	}
	for dialect, want := range wantOffset {
		c, _ := comOffset.Compile(orm.Select, orm.CompileOptions{Dialect: dialect})
		if c.SQL != want {
			t.Fatalf("offset %s:\n got %q\nwant %q", dialect, c.SQL, want)
		}
	}

	// Só limit (sem offset): SQL Server usa TOP; os demais mantêm sua cláusula.
	soLimit := m.All().Limit(5)
	wantLimit := map[orm.Dialect]string{
		orm.Oracle:    `SELECT "ID", "NOME", "IDADE", "ATIVO" FROM "USERS" FETCH FIRST 5 ROWS ONLY`,
		orm.Postgres:  `SELECT "id", "nome", "idade", "ativo" FROM "users" LIMIT 5`,
		orm.MySQL:     "SELECT `id`, `nome`, `idade`, `ativo` FROM `users` LIMIT 5",
		orm.SQLServer: `SELECT TOP 5 [id], [nome], [idade], [ativo] FROM [users]`,
	}
	for dialect, want := range wantLimit {
		c, _ := soLimit.Compile(orm.Select, orm.CompileOptions{Dialect: dialect})
		if c.SQL != want {
			t.Fatalf("limit %s:\n got %q\nwant %q", dialect, c.SQL, want)
		}
	}
}

// TestExistsPorDialeto cobre a variação do "limitar a 1" no exists.
func TestExistsPorDialeto(t *testing.T) {
	m, f := testModel()
	q := m.Where(f.Id.Equal(7))
	casos := map[orm.Dialect]string{
		orm.Oracle:    `SELECT 1 FROM "USERS" WHERE "ID" = :1 FETCH FIRST 1 ROWS ONLY`,
		orm.Postgres:  `SELECT 1 FROM "users" WHERE "id" = $1 LIMIT 1`,
		orm.MySQL:     "SELECT 1 FROM `users` WHERE `id` = ? LIMIT 1",
		orm.SQLServer: `SELECT TOP 1 1 FROM [users] WHERE [id] = @p1`,
	}
	for dialect, want := range casos {
		c, _ := q.Compile(orm.Exists, orm.CompileOptions{Dialect: dialect})
		if c.SQL != want {
			t.Fatalf("exists %s:\n got %q\nwant %q", dialect, c.SQL, want)
		}
	}
}

// TestSchemaQualificado: Oracle/PG/SQL Server qualificam com schema; MySQL não.
func TestSchemaQualificado(t *testing.T) {
	m, f := testModel()
	q := m.Where(f.Id.Equal(1))
	casos := map[orm.Dialect]string{
		orm.Oracle:    `"APP"."USERS"`,
		orm.Postgres:  `"app"."users"`,
		orm.MySQL:     "`users`",
		orm.SQLServer: `[app].[users]`,
	}
	for dialect, want := range casos {
		c, _ := q.Compile(orm.Select, orm.CompileOptions{Dialect: dialect, Schema: "app"})
		if !contains(c.SQL, "FROM "+want) {
			t.Fatalf("schema %s: FROM esperado %q em %q", dialect, want, c.SQL)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestFiltroDataEIntervalo(t *testing.T) {
	dt := orm.Field{Name: "Criado", Entity: "users", Table: "users", Column: "criado_em", DataType: "datetime"}
	d := orm.DateFilter(dt)
	m := orm.NewModel[orm.Record](orm.EntityFields{Name: "users", Fields: []orm.Field{dt}}, orm.ScanRecords)

	// intervalo fechado com time.Time
	ini := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	c, err := m.Where(d.Between(ini, fim)).Compile(orm.Select, orm.CompileOptions{Dialect: orm.Oracle})
	if err != nil {
		t.Fatal(err)
	}
	if want := `WHERE "CRIADO_EM" BETWEEN :1 AND :2`; !contains(c.SQL, want) {
		t.Fatalf("SQL sem %q: %s", want, c.SQL)
	}
	if len(c.Args) != 2 || c.Args[0] != any(ini) {
		t.Fatalf("Args: %#v", c.Args)
	}

	// valor CRU (query string) convertido para time.Time, não string
	c2, _ := m.Where(orm.Filter(d, orm.GreaterOrEqual, "2024-06-15")).Compile(orm.Select, orm.CompileOptions{Dialect: orm.Oracle})
	if len(c2.Args) != 1 {
		t.Fatalf("esperava 1 arg: %#v", c2.Args)
	}
	if _, ok := c2.Args[0].(time.Time); !ok {
		t.Fatalf("arg deveria ser time.Time, veio %T", c2.Args[0])
	}
	// data inválida é ignorada (filtro dinâmico)
	c3, _ := m.Where(orm.Filter(d, orm.Equal, "não-é-data")).Compile(orm.Select, orm.CompileOptions{Dialect: orm.Oracle})
	if contains(c3.SQL, "WHERE") {
		t.Fatalf("data inválida não deveria filtrar: %s", c3.SQL)
	}
}
