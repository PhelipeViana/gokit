package migraterun

// O seeder é soberano sobre os dados.
//
// Decisão do usuário, e substitui o invariante anterior ("seed nunca sobrescreve linha que
// não é dele"). Onde existe seeder, o seeder manda: linha divergente é SOBRESCRITA em vez de
// abortar o run.
//
// O custo foi aceito com ele à vista: dado da aplicação naquela linha é substituído. O que
// este arquivo garante é o RASTRO — cada sobrescrita sai no relatório com o valor antigo e o
// novo, e vai para o monitor. Não impede a perda; torna impossível ela passar em silêncio.

import (
	"fmt"
	"strings"
)

// seedSobrescritas acumula as linhas trocadas por decisão de soberania.
//
// Variável de pacote pela mesma razão de indicesRedundantes: o executor de operação não
// carrega um coletor, e propagar um tocaria dezenas de assinaturas por causa de um relato.
var seedSobrescritas []string

func avisarSeedSobrescrita(tabela, chave, diferenca string) {
	seedSobrescritas = append(seedSobrescritas,
		fmt.Sprintf("%s %s — %s", strings.ToLower(tabela), chave, diferenca))
}

// SeedSobrescritas devolve e ZERA a lista acumulada.
func SeedSobrescritas() []string {
	saida := seedSobrescritas
	seedSobrescritas = nil
	return saida
}

// seedSubstituicoes acumula as tabelas SEM CHAVE cujo conteúdo foi substituído inteiro.
var seedSubstituicoes []string

func avisarSeedSubstituicao(tabela string, apagadas, inseridas int) {
	seedSubstituicoes = append(seedSubstituicoes,
		fmt.Sprintf("%s — %d linha(s) apagada(s), %d inserida(s)",
			strings.ToLower(tabela), apagadas, inseridas))
}

// SeedSubstituicoes devolve e ZERA a lista acumulada.
func SeedSubstituicoes() []string {
	saida := seedSubstituicoes
	seedSubstituicoes = nil
	return saida
}
