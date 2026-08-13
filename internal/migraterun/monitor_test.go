package migraterun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O arquivo do monitor é contrato: o playground vai lê-lo, e a equipe vai comparar
// execuções para ver se a lista diminuiu. Nome de campo e ordem são parte disso.
func TestMonitorGravaRegistroEstavel(t *testing.T) {
	raiz := t.TempDir()
	monitor := NovoMonitor("migrate import", "proxima", "sqlserver")

	// De propósito fora de ordem: a gravação tem de ordenar.
	monitor.Registrar(Ocorrencia{Tipo: OcorrenciaNaoLido, Tabela: "zebra", Objeto: "ix_z"})
	monitor.Registrar(Ocorrencia{Tipo: OcorrenciaIdentidadeForaDaChave, Tabela: "alfa", Coluna: "seq"})
	monitor.Registrar(Ocorrencia{Tipo: OcorrenciaNaoLido, Tabela: "alfa", Objeto: "ix_a"})

	caminho, err := monitor.Escrever(raiz, filepath.Join("internal", "gokit", "docs"))
	if err != nil {
		t.Fatal(err)
	}
	if caminho != "internal/gokit/docs/monitor-import.json" {
		t.Fatalf("caminho relativo veio %q", caminho)
	}

	dados, err := os.ReadFile(filepath.Join(raiz, filepath.FromSlash(caminho)))
	if err != nil {
		t.Fatal(err)
	}

	var lido Monitor
	if err := json.Unmarshal(dados, &lido); err != nil {
		t.Fatalf("o registro tem de ser JSON válido: %v", err)
	}
	if lido.Total != 3 || lido.PorTipo[OcorrenciaNaoLido] != 2 {
		t.Fatalf("contagens vieram %+v", lido)
	}
	// Ordenado por tipo e depois por tabela: identidade_fora_da_chave < nao_lido.
	esperado := []string{"alfa", "alfa", "zebra"}
	for posicao, ocorrencia := range lido.Ocorrencias {
		if ocorrencia.Tabela != esperado[posicao] {
			t.Fatalf("ordem instável: posição %d é %q, esperava %q", posicao, ocorrencia.Tabela, esperado[posicao])
		}
	}
	// Os nomes dos campos são contrato com o playground.
	for _, chave := range []string{`"tipo"`, `"tabela"`, `"por_tipo"`, `"ocorrencias"`, `"dialeto"`} {
		if !strings.Contains(string(dados), chave) {
			t.Errorf("o registro deveria conter a chave %s", chave)
		}
	}
}

// Sem ocorrência não existe arquivo: import limpo não deixa registro para trás nem
// faz alguém revisar lista vazia.
func TestMonitorVazioNaoGravaArquivo(t *testing.T) {
	raiz := t.TempDir()
	monitor := NovoMonitor("migrate import", "mysql", "mysql")

	caminho, err := monitor.Escrever(raiz, "internal/gokit/docs")
	if err != nil {
		t.Fatal(err)
	}
	if caminho != "" {
		t.Fatalf("não deveria gravar nada, devolveu %q", caminho)
	}
	if _, err := os.Stat(filepath.Join(raiz, "internal", "gokit", "docs", nomeDoMonitor)); !os.IsNotExist(err) {
		t.Fatal("o arquivo não deveria existir")
	}
}

// Monitor nulo aceita registro: quem produz ocorrência não pode ser obrigado a
// checar, senão a checagem é esquecida em algum caminho.
func TestMonitorNuloNaoQuebra(t *testing.T) {
	var monitor *Monitor
	monitor.Registrar(Ocorrencia{Tipo: OcorrenciaNaoLido})
	if !monitor.Vazio() {
		t.Fatal("monitor nulo é vazio")
	}
}

// O tipo só é registrado quando REALMENTE não tem equivalente: registrar o que foi
// traduzido corretamente encheria a lista de ruído e ninguém revisaria.
func TestSoRegistraTipoRealmenteDesconhecido(t *testing.T) {
	monitor := NovoMonitor("teste", "x", "sqlserver")
	registrarTipoDegradado(monitor, "t", ColunaDoBanco{Nome: "a", Tipo: "varchar", Tamanho: 20}, "sqlserver", ".Varchar(20)")
	registrarTipoDegradado(monitor, "t", ColunaDoBanco{Nome: "b", Tipo: "xml"}, "sqlserver", ".Text()")
	if monitor.Total != 0 {
		t.Fatalf("varchar e xml são conhecidos; registrou %+v", monitor.Ocorrencias)
	}

	registrarTipoDegradado(monitor, "t", ColunaDoBanco{Nome: "c", Tipo: "geography"}, "sqlserver", ".Text()")
	if monitor.Total != 1 || monitor.Ocorrencias[0].Origem != "geography" {
		t.Fatalf("geography deveria ser registrado, veio %+v", monitor.Ocorrencias)
	}
}
