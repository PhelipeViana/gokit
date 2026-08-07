# Suíte de regressão do contrato de seed e ID fixo.
#
# Cria uma migration própria com timestamp 9999 (roda depois de todo o corpus),
# manipula o Seeder() entre os cenários e confere o resultado nos quatro
# bancos. No fim remove tudo que criou.
#
# Roda em cima de um banco já migrado: só a migration do lab é aplicada a cada
# cenário, então o ciclo é de segundos e não de minutos.

$ContractId = "9999_01_01_000000"
$ContractTable = "lab_contrato"

function Get-ContractFile {
    return Join-Path $TestProject "database\migrations\create_table\${ContractId}_lab_contrato.go"
}

function Set-ContractSeeder([string[]]$rows) {
    $body = ($rows | ForEach-Object { "`t`t$_," }) -join "`n"
    $content = @"
package migrations

import (
	migrate "gokit/migration"
)

// Gerado pelo testlab (testlab/contract.ps1). Não editar à mão.
func Migration() migrate.Definition {
	return migrate.Define(
		migrate.CreateTable("$ContractTable",
			migrate.Col("id").Integer().PrimaryKey().AutoIncrement(),
			migrate.Col("nome").Varchar(120),
		).Alias("$ContractTable"),
	)
}

func Seeder() migrate.Rows {
	return migrate.Rows{
$body
	}
}
"@
    $utf8 = New-Object System.Text.UTF8Encoding($false)
    [IO.File]::WriteAllText((Get-ContractFile), $content, $utf8)
}

function Remove-ContractState($dialect) {
    $drop = switch ($dialect) {
        "oracle" { "begin execute immediate 'drop table $ContractTable cascade constraints purge'; exception when others then null; end;" }
        "postgres" { "DROP TABLE IF EXISTS $ContractTable CASCADE" }
        "mysql" { "DROP TABLE IF EXISTS $ContractTable" }
        "sqlserver" { "IF OBJECT_ID('$ContractTable') IS NOT NULL DROP TABLE $ContractTable" }
    }
    try { Invoke-Sql $dialect $drop | Out-Null } catch {}
    try { Invoke-Sql $dialect "DELETE FROM migrations_gokit WHERE migration='$ContractId'" | Out-Null } catch {}
}

function Clear-ContractHistory($dialect) {
    Invoke-Sql $dialect "DELETE FROM migrations_gokit WHERE migration='$ContractId'" | Out-Null
}

# --------------------------------------------------------------------------

$script:Passed = 0
$script:Failed = 0

function Assert-That($dialect, $label, $condition, $detail) {
    if ($condition) {
        $script:Passed++
        Write-Host ("      {0,-10} PASS  {1}" -f $dialect, $label) -ForegroundColor Green
    }
    else {
        $script:Failed++
        Write-Host ("      {0,-10} FAIL  {1} — {2}" -f $dialect, $label, $detail) -ForegroundColor Red
    }
}

function Invoke-ContractRun($dialect) {
    Clear-ContractHistory $dialect
    return Invoke-Timed $dialect @("migrate", "run")
}

function Get-Scalar($dialect, $query) {
    return Invoke-SqlScalar $dialect $query
}

# --------------------------------------------------------------------------

function Cmd-Contract($name) {
    if (-not (Test-Containers)) { return }
    $dialects = Resolve-Dialects $name
    $script:Passed = 0
    $script:Failed = 0

    Write-Step "Contrato de seed · build"
    Cmd-Build

    try {
        # -- 1 ------------------------------------------------------------
        Write-Step "1. Seed fixo aplicado na criação da tabela"
        Set-ContractSeeder @('{"id": 10, "nome": "Ana"}', '{"id": 1, "nome": "Silva"}')
        foreach ($d in $dialects) {
            Remove-ContractState $d
            $run = Invoke-ContractRun $d
            $total = Get-Scalar $d "SELECT COUNT(*) FROM $ContractTable"
            $nome = Get-Scalar $d "SELECT nome FROM $ContractTable WHERE id = 10"
            Assert-That $d "2 linhas inseridas" ($total -eq "2") "veio '$total'"
            Assert-That $d "id=10 é 'Ana'" ($nome -eq "Ana") "veio '$nome'"
        }

        # -- 2 ------------------------------------------------------------
        Write-Step "2. Sequência pula o ID fixo (app insere e recebe 11)"
        foreach ($d in $dialects) {
            Invoke-Sql $d "INSERT INTO $ContractTable (nome) VALUES ('App')" | Out-Null
            $novo = Get-Scalar $d "SELECT id FROM $ContractTable WHERE nome = 'App'"
            Assert-That $d "app recebeu id=11" ($novo -eq "11") "veio '$novo'"
        }

        # -- 3 ------------------------------------------------------------
        Write-Step "3. Reexecução do seed inalterado é no-op"
        foreach ($d in $dialects) {
            $run = Invoke-ContractRun $d
            $total = Get-Scalar $d "SELECT COUNT(*) FROM $ContractTable"
            Assert-That $d "não duplicou" ($total -eq "3") "veio '$total'"
            Assert-That $d "reportou 0 inseridas" ($run.Output -match "0 inserida") "saída: $($run.Output)"
        }

        # -- 4 ------------------------------------------------------------
        Write-Step "4. ID ocupado pela aplicação: erro, sem sobrescrever"
        Set-ContractSeeder @('{"id": 10, "nome": "Ana"}', '{"id": 1, "nome": "Silva"}', '{"id": 11, "nome": "Conflito"}')
        foreach ($d in $dialects) {
            $run = Invoke-ContractRun $d
            $nome = Get-Scalar $d "SELECT nome FROM $ContractTable WHERE id = 11"
            Assert-That $d "run falhou" $run.Failed "deveria ter falhado"
            Assert-That $d "linha da app intacta" ($nome -eq "App") "veio '$nome'"
        }

        # -- 5 ------------------------------------------------------------
        Write-Step "5. Atomicidade: insert válido antes do conflito é revertido"
        Set-ContractSeeder @('{"id": 10, "nome": "Ana"}', '{"id": 1, "nome": "Silva"}',
            '{"id": 500, "nome": "Some No Rollback"}', '{"id": 11, "nome": "Conflito"}')
        foreach ($d in $dialects) {
            $run = Invoke-ContractRun $d
            $orfao = Get-Scalar $d "SELECT COUNT(*) FROM $ContractTable WHERE id = 500"
            Assert-That $d "run falhou" $run.Failed "deveria ter falhado"
            Assert-That $d "id=500 não ficou gravado" ($orfao -eq "0") "veio '$orfao'"
        }

        # -- 6 ------------------------------------------------------------
        Write-Step "6. ID fixo alto entra e a sequência nunca reemite"
        Set-ContractSeeder @('{"id": 10, "nome": "Ana"}', '{"id": 1, "nome": "Silva"}', '{"id": 144, "nome": "Fixo Alto"}')
        foreach ($d in $dialects) {
            $run = Invoke-ContractRun $d
            $alto = Get-Scalar $d "SELECT nome FROM $ContractTable WHERE id = 144"
            Assert-That $d "run passou" (-not $run.Failed) "saída: $($run.Output)"
            Assert-That $d "id=144 inserido" ($alto -eq "Fixo Alto") "veio '$alto'"

            Invoke-Sql $d "INSERT INTO $ContractTable (nome) VALUES ('Depois')" | Out-Null
            $depois = [int64](Get-Scalar $d "SELECT id FROM $ContractTable WHERE nome = 'Depois'")
            # O invariante é "nunca reemitir", não "ser exatamente 145": o SQL
            # Server não desfaz o avanço do identity num rollback, então lá o
            # próximo é maior. Gap é inofensivo, reemissão não seria.
            Assert-That $d "próximo id > 144" ($depois -gt 144) "veio '$depois'"
        }
    }
    finally {
        Write-Step "Limpeza"
        foreach ($d in $dialects) { Remove-ContractState $d }
        Remove-Item (Get-ContractFile) -ErrorAction SilentlyContinue
        Write-Ok "migration e tabela do lab removidas"
    }

    Write-Step "Resultado"
    Write-Host ("  {0} passaram, {1} falharam" -f $script:Passed, $script:Failed)
    if ($script:Failed -gt 0) { Write-Fail "contrato quebrado"; exit 1 }
    Write-Ok "contrato de seed/ID fixo íntegro nos $($dialects.Count) dialeto(s)"
}
