package migraterun

// Sonda de classificação de erro contra os QUATRO bancos de verdade.
//
// Por que um teste, e não um programa solto: a tabela de marcadores do orm/erros.go
// foi escrita à mão, e mensagem de driver é exatamente o tipo de coisa que não se
// supõe. Este teste provoca cada classe de erro no banco, captura a mensagem crua e
// confere o veredito do ClassifyError contra ela.
//
// Roda só com GOKIT_SONDA=1 e com os containers de pé; sem isso é skip, para não
// quebrar `go test ./...` de quem não tem banco. A tabela de mensagens cruas é
// impressa mesmo quando tudo passa: ela é o registro do que os drivers realmente
// dizem, e é o que permite julgar um marcador novo sem ter os quatro na mão.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PhelipeViana/gokit/orm"
)

type sondaBanco struct {
	dialeto string
	url     string
	// prefixo é o schema qualificado, quando o dialeto exige.
	criarPai   string
	criarFilho string
}

func sondasConfiguradas() []sondaBanco {
	return []sondaBanco{
		{
			dialeto: "mysql",
			url:     "gokit_user:gokit_password@tcp(127.0.0.1:20590)/gokit_onboarding?parseTime=true",
			criarPai: `CREATE TABLE sonda_pai (
				id INT NOT NULL PRIMARY KEY,
				nome VARCHAR(10) NOT NULL,
				idade INT NULL,
				CONSTRAINT uk_sonda_nome UNIQUE (nome),
				CONSTRAINT chk_sonda_idade CHECK (idade >= 0))`,
			criarFilho: `CREATE TABLE sonda_filho (
				id INT NOT NULL PRIMARY KEY,
				pai_id INT NOT NULL,
				CONSTRAINT fk_sonda_pai FOREIGN KEY (pai_id) REFERENCES sonda_pai(id))`,
		},
		{
			dialeto: "postgres",
			url:     "postgres://gokit_user:gokit_password@127.0.0.1:24443/gokit_onboarding?sslmode=disable",
			criarPai: `CREATE TABLE sonda_pai (
				id INT NOT NULL PRIMARY KEY,
				nome VARCHAR(10) NOT NULL,
				idade INT NULL,
				CONSTRAINT uk_sonda_nome UNIQUE (nome),
				CONSTRAINT chk_sonda_idade CHECK (idade >= 0))`,
			criarFilho: `CREATE TABLE sonda_filho (
				id INT NOT NULL PRIMARY KEY,
				pai_id INT NOT NULL,
				CONSTRAINT fk_sonda_pai FOREIGN KEY (pai_id) REFERENCES sonda_pai(id))`,
		},
		{
			dialeto: "oracle",
			url:     "oracle://gokit_user:gokit_password@127.0.0.1:22889/FREEPDB1",
			criarPai: `CREATE TABLE sonda_pai (
				id NUMBER(10) NOT NULL PRIMARY KEY,
				nome VARCHAR2(10) NOT NULL,
				idade NUMBER(10) NULL,
				CONSTRAINT uk_sonda_nome UNIQUE (nome),
				CONSTRAINT chk_sonda_idade CHECK (idade >= 0))`,
			criarFilho: `CREATE TABLE sonda_filho (
				id NUMBER(10) NOT NULL PRIMARY KEY,
				pai_id NUMBER(10) NOT NULL,
				CONSTRAINT fk_sonda_pai FOREIGN KEY (pai_id) REFERENCES sonda_pai(id))`,
		},
		{
			dialeto: "sqlserver",
			url:     "sqlserver://sa:Gokit_password123!@127.0.0.1:28401?database=gokit_onboarding&encrypt=disable",
			criarPai: `CREATE TABLE sonda_pai (
				id INT NOT NULL PRIMARY KEY,
				nome VARCHAR(10) NOT NULL,
				idade INT NULL,
				CONSTRAINT uk_sonda_nome UNIQUE (nome),
				CONSTRAINT chk_sonda_idade CHECK (idade >= 0))`,
			criarFilho: `CREATE TABLE sonda_filho (
				id INT NOT NULL PRIMARY KEY,
				pai_id INT NOT NULL,
				CONSTRAINT fk_sonda_pai FOREIGN KEY (pai_id) REFERENCES sonda_pai(id))`,
		},
	}
}

// provocacao é um caso: o SQL que faz o banco recusar e a classe esperada.
type provocacao struct {
	nome     string
	esperada error
	// sql por dialeto quando divergem; "" usa o padrao.
	padrao     string
	porDialeto map[string]string
}

func provocacoes() []provocacao {
	return []provocacao{
		{
			nome:     "duplicidade/chave primária",
			esperada: orm.ErrDuplicate,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (1, 'outro', 1)`,
		},
		{
			nome:     "duplicidade/chave única",
			esperada: orm.ErrDuplicate,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (99, 'ana', 1)`,
		},
		{
			nome:     "obrigatoriedade/NOT NULL",
			esperada: orm.ErrNotNull,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (98, NULL, 1)`,
		},
		{
			nome:     "tamanho/valor maior que a coluna",
			esperada: orm.ErrTooLong,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (97, 'texto muito maior que dez', 1)`,
		},
		{
			nome:     "CHECK/valor recusado",
			esperada: orm.ErrCheck,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (96, 'chk', -5)`,
		},
		{
			nome:     "FK/pai inexistente",
			esperada: orm.ErrForeignKey,
			padrao:   `INSERT INTO sonda_filho (id, pai_id) VALUES (1, 4242)`,
		},
		{
			nome:     "FK/apagar pai com filho",
			esperada: orm.ErrForeignKey,
			padrao:   `DELETE FROM sonda_pai WHERE id = 1`,
		},
		// As quatro abaixo entraram como "sem classe" e a sonda mostrou que valia
		// classificar: as duas primeiras são erro de programa, as duas últimas são dado
		// do cliente e merecem 422 em vez de 500.
		{
			nome:     "tabela inexistente",
			esperada: orm.ErrUndefinedObject,
			padrao:   `INSERT INTO sonda_que_nao_existe (id) VALUES (1)`,
		},
		{
			nome:     "coluna inexistente",
			esperada: orm.ErrUndefinedObject,
			padrao:   `INSERT INTO sonda_pai (id, coluna_que_nao_existe) VALUES (95, 1)`,
		},
		{
			nome:     "texto em coluna numérica",
			esperada: orm.ErrInvalidValue,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (94, 'num', 'abc')`,
		},
		{
			nome:     "estouro de precisão numérica",
			esperada: orm.ErrOutOfRange,
			padrao:   `INSERT INTO sonda_pai (id, nome, idade) VALUES (93, 'ovf', 999999999999999)`,
		},
	}
}

func (p provocacao) sqlPara(dialeto string) string {
	if especifico, tem := p.porDialeto[dialeto]; tem {
		return especifico
	}
	return p.padrao
}

func TestSondaDeErrosNosQuatroBancos(t *testing.T) {
	if os.Getenv("GOKIT_SONDA") != "1" {
		t.Skip("sonda de banco: rode com GOKIT_SONDA=1 e os containers de pé")
	}

	type linha struct {
		dialeto  string
		caso     string
		esperada error
		obtida   error
		bruta    string
	}
	var tabela []linha

	for _, sonda := range sondasConfiguradas() {
		t.Run(sonda.dialeto, func(t *testing.T) {
			driver := sonda.dialeto
			switch sonda.dialeto {
			case "postgres":
				driver = "pgx"
			case "oracle":
				driver = "oracle"
			}
			db, err := sql.Open(driver, sonda.url)
			if err != nil {
				t.Fatalf("abrir %s: %v", sonda.dialeto, err)
			}
			defer db.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				t.Skipf("%s indisponível: %v", sonda.dialeto, err)
			}

			// Monta o cenário do zero. A ordem do drop é filho antes de pai.
			for _, tabelaSonda := range []string{"sonda_filho", "sonda_pai"} {
				_, _ = db.ExecContext(ctx, "DROP TABLE "+tabelaSonda)
			}
			for _, ddl := range []string{sonda.criarPai, sonda.criarFilho} {
				if _, err := db.ExecContext(ctx, ddl); err != nil {
					t.Fatalf("%s: criar cenário: %v", sonda.dialeto, err)
				}
			}
			semear := []string{
				`INSERT INTO sonda_pai (id, nome, idade) VALUES (1, 'ana', 30)`,
				`INSERT INTO sonda_filho (id, pai_id) VALUES (10, 1)`,
			}
			for _, ins := range semear {
				if _, err := db.ExecContext(ctx, ins); err != nil {
					t.Fatalf("%s: semear: %v", sonda.dialeto, err)
				}
			}

			for _, caso := range provocacoes() {
				_, err := db.ExecContext(ctx, caso.sqlPara(sonda.dialeto))
				if err == nil {
					t.Errorf("%s / %s: o banco ACEITOU o que deveria recusar", sonda.dialeto, caso.nome)
					continue
				}
				classificado := orm.ClassifyError(err)
				var comoBanco orm.DatabaseError
				var obtida error
				if ok := asDatabaseError(classificado, &comoBanco); ok {
					obtida = comoBanco.Classe
				}
				tabela = append(tabela, linha{
					dialeto: sonda.dialeto, caso: caso.nome,
					esperada: caso.esperada, obtida: obtida,
					bruta: umaLinha(err.Error()),
				})
				if caso.esperada != nil && obtida != caso.esperada {
					t.Errorf("%s / %s\n  esperada: %v\n  obtida:   %v\n  mensagem: %s",
						sonda.dialeto, caso.nome, caso.esperada, obtida, umaLinha(err.Error()))
				}
			}

			for _, tabelaSonda := range []string{"sonda_filho", "sonda_pai"} {
				_, _ = db.ExecContext(ctx, "DROP TABLE "+tabelaSonda)
			}
		})
	}

	// O registro sai sempre: é ele que permite julgar marcador novo sem os quatro
	// bancos na mão.
	var relatorio strings.Builder
	relatorio.WriteString("\n== mensagens cruas dos quatro drivers ==\n")
	for _, l := range tabela {
		veredito := "SEM CLASSE"
		if l.obtida != nil {
			veredito = l.obtida.Error()
		}
		relatorio.WriteString(fmt.Sprintf("[%-9s] %-42s -> %s\n            %s\n",
			l.dialeto, l.caso, veredito, l.bruta))
	}
	t.Log(relatorio.String())
}

func asDatabaseError(err error, destino *orm.DatabaseError) bool {
	if convertido, ok := err.(orm.DatabaseError); ok {
		*destino = convertido
		return true
	}
	return false
}

func umaLinha(texto string) string {
	texto = strings.ReplaceAll(texto, "\r\n", " | ")
	texto = strings.ReplaceAll(texto, "\n", " | ")
	return strings.TrimSpace(texto)
}
