# Laboratório de teste local do GoKit.
#
# Compila, publica o binário no projeto de teste e exercita as migrations nos
# quatro dialetos sem precisar lembrar de porta, senha ou nome de container.
#
#   .\testlab\lab.ps1 help

param(
    [Parameter(Position = 0)][string]$Command = "help",
    [Parameter(Position = 1)][string]$Arg1,
    [Parameter(Position = 2)][string]$Arg2
)

$ErrorActionPreference = "Stop"

# --------------------------------------------------------------------------
# Configuração
# --------------------------------------------------------------------------

$RepoRoot = Split-Path -Parent $PSScriptRoot

# Projeto de teste: sobrescreva com $env:GOKIT_TESTLAB_PROJECT se o seu não
# estiver ao lado do repositório.
$TestProject = $env:GOKIT_TESTLAB_PROJECT
if (-not $TestProject) {
    $TestProject = Join-Path (Split-Path -Parent $RepoRoot) "prevcontas_test"
}
$Binary = Join-Path $TestProject "gokit-windows-amd64.exe"

# Tudo que muda entre os bancos mora aqui e em nenhum outro lugar.
$Dialects = [ordered]@{
    oracle    = @{
        Container = "prevcontas_test_oracle"
        Env       = @{ DB_DIALECT = "oracle"; DB_SCHEMA = "PREVCONTAS_TEST" }
    }
    postgres  = @{
        Container = "prevcontas_test_postgres"
        Env       = @{ DB_DIALECT = "postgres"; DB_SCHEMA = "public" }
    }
    mysql     = @{
        Container = "prevcontas_test_mysql"
        Env       = @{ DB_DIALECT = "mysql"; DB_SCHEMA = "prevcontas_test" }
    }
    sqlserver = @{
        Container = "prevcontas_test_sqlserver"
        Env       = @{ DB_DIALECT = "sqlserver"; DB_SCHEMA = "dbo"; DB_USER = "sa"; DB_PASSWORD = "Prevcontas_test123!" }
    }
}

# --------------------------------------------------------------------------
# Saída
# --------------------------------------------------------------------------

function Write-Step($text) { Write-Host "`n== $text" -ForegroundColor Cyan }
function Write-Ok($text) { Write-Host "  [ok] $text" -ForegroundColor Green }
function Write-Fail($text) { Write-Host "  [!!] $text" -ForegroundColor Red }
function Write-Note($text) { Write-Host "  $text" -ForegroundColor DarkGray }

function Resolve-Dialects($name) {
    if (-not $name -or $name -eq "all") { return @($Dialects.Keys) }
    if (-not $Dialects.Contains($name)) {
        throw "Dialeto desconhecido: '$name'. Use: $($Dialects.Keys -join ', ') ou 'all'."
    }
    return @($name)
}

# --------------------------------------------------------------------------
# Banco
# --------------------------------------------------------------------------

function Invoke-Sql($dialect, $query) {
    $container = $Dialects[$dialect].Container
    switch ($dialect) {
        "oracle" {
            $script = "set heading off feedback off pagesize 0 linesize 4000 trimspool on;`n$query;`nexit;`n"
            $out = $script | docker exec -i $container sqlplus -s prevcontas_test/prevcontas_test_123@localhost:1521/FREEPDB1
        }
        "postgres" {
            $out = docker exec -i $container psql -U prevcontas_test -d prevcontas_test -t -A -F "|" -c $query
        }
        "mysql" {
            $out = docker exec -i $container mysql -uprevcontas_test -pprevcontas_test_123 -D prevcontas_test -N -B -e $query
        }
        "sqlserver" {
            $out = docker exec -i $container /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P "Prevcontas_test123!" -C -d prevcontas_test "-h-1" -W -s "|" -Q "SET NOCOUNT ON; $query"
        }
    }
    # Ruído previsível dos clientes: aviso de senha no MySQL, linhas vazias do
    # SQL*Plus. Nenhum deles é resultado.
    return ($out | Where-Object {
            $_ -and
            ($_ -notmatch "Using a password on the command line") -and
            ($_.Trim() -ne "")
        })
}

function Invoke-SqlScalar($dialect, $query) {
    $rows = Invoke-Sql $dialect $query
    if (-not $rows) { return "" }
    return ("$($rows[0])").Trim()
}

function Reset-Schema($dialect) {
    switch ($dialect) {
        "oracle" {
            # O Oracle não tem DROP SCHEMA para o próprio usuário: é objeto a objeto.
            $block = @"
begin
 for r in (select view_name from user_views) loop execute immediate 'drop view "'||r.view_name||'"'; end loop;
 for r in (select table_name from user_tables) loop execute immediate 'drop table "'||r.table_name||'" cascade constraints purge'; end loop;
 for r in (select sequence_name from user_sequences) loop execute immediate 'drop sequence "'||r.sequence_name||'"'; end loop;
end;
/
"@
            $script = "set heading off feedback off pagesize 0;`n$block`nexit;`n"
            $script | docker exec -i prevcontas_test_oracle sqlplus -s prevcontas_test/prevcontas_test_123@localhost:1521/FREEPDB1 | Out-Null
        }
        "postgres" {
            Invoke-Sql $dialect "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO prevcontas_test;" | Out-Null
        }
        "mysql" {
            docker exec -i prevcontas_test_mysql mysql -uroot -pprevcontas_test_root_123 -e "DROP DATABASE IF EXISTS prevcontas_test; CREATE DATABASE prevcontas_test; GRANT ALL ON prevcontas_test.* TO 'prevcontas_test'@'%';" | Out-Null
        }
        "sqlserver" {
            docker exec -i prevcontas_test_sqlserver /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P "Prevcontas_test123!" -C -Q "IF DB_ID('prevcontas_test') IS NOT NULL BEGIN ALTER DATABASE prevcontas_test SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE prevcontas_test; END; CREATE DATABASE prevcontas_test;" | Out-Null
        }
    }
}

function Test-Containers {
    $running = docker ps --format "{{.Names}}"
    $missing = @()
    foreach ($name in $Dialects.Keys) {
        if ($running -notcontains $Dialects[$name].Container) { $missing += $Dialects[$name].Container }
    }
    if ($missing.Count -gt 0) {
        Write-Fail "Containers fora do ar: $($missing -join ', ')"
        Write-Note "Suba com: docker compose up -d  (dentro de $TestProject)"
        return $false
    }
    return $true
}

# --------------------------------------------------------------------------
# GoKit
# --------------------------------------------------------------------------

function Invoke-Gokit($dialect, $gokitArgs) {
    $saved = @{}
    foreach ($key in $Dialects[$dialect].Env.Keys) {
        $saved[$key] = [Environment]::GetEnvironmentVariable($key)
        Set-Item -Path "env:$key" -Value $Dialects[$dialect].Env[$key]
    }
    try {
        Push-Location $TestProject
        try { & $Binary @gokitArgs } finally { Pop-Location }
    }
    finally {
        foreach ($key in $saved.Keys) {
            if ($null -eq $saved[$key]) { Remove-Item "env:$key" -ErrorAction SilentlyContinue }
            else { Set-Item -Path "env:$key" -Value $saved[$key] }
        }
    }
}

function Invoke-Timed($dialect, $gokitArgs) {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $output = Invoke-Gokit $dialect $gokitArgs
    $watch.Stop()
    return [pscustomobject]@{
        Output = ($output -join "`n")
        Ms     = [int]$watch.ElapsedMilliseconds
        Failed = ($output -match "ERRO|Erro:").Count -gt 0
    }
}

# --------------------------------------------------------------------------
# Comandos
# --------------------------------------------------------------------------

function Cmd-Build {
    Write-Step "Compilando e publicando em $Binary"
    Push-Location $RepoRoot
    try {
        # CommitHash=development desliga o auto-update. Sem isso o binário
        # buscaria a versão do GitHub e sobrescreveria o build local.
        $stamp = Get-Date -Format "dd/MM/yyyy HH:mm"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        go build -ldflags "-X main.CommitHash=development -X 'main.Version=testlab ($stamp)'" -o $Binary .
        if ($LASTEXITCODE -ne 0) { throw "go build falhou" }
    }
    finally {
        Remove-Item env:GOOS -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
        Pop-Location
    }
    Write-Ok "binário publicado ($([int]((Get-Item $Binary).Length / 1MB)) MB)"
}

function Cmd-Reset($name) {
    foreach ($dialect in (Resolve-Dialects $name)) {
        Write-Host ("  limpando {0,-10}" -f $dialect) -NoNewline
        Reset-Schema $dialect
        Write-Host " ok" -ForegroundColor Green
    }
}

function Cmd-Validate {
    Write-Step "Pré-validação do corpus (não toca no banco)"
    Invoke-Gokit "postgres" @("migrate", "validate")
}

function Cmd-Run($name) {
    foreach ($dialect in (Resolve-Dialects $name)) {
        Write-Step "migrate run · $dialect"
        $result = Invoke-Timed $dialect @("migrate", "run")
        Write-Host $result.Output
        Write-Note "$($result.Ms) ms"
    }
}

function Cmd-Sql($name, $query) {
    if (-not $query) { throw "Uso: lab.ps1 sql <dialeto> `"<query>`"" }
    foreach ($dialect in (Resolve-Dialects $name)) {
        Write-Host ("-- {0}" -f $dialect) -ForegroundColor Cyan
        Invoke-Sql $dialect $query
    }
}

function Cmd-Matrix {
    if (-not (Test-Containers)) { return }
    Cmd-Build
    Write-Step "Pré-validação"
    $validation = Invoke-Gokit "postgres" @("migrate", "validate")
    Write-Host ($validation -join "`n")
    if (($validation -match "✗").Count -gt 0) {
        Write-Fail "corpus inválido — matriz abortada"
        return
    }

    $report = @()
    Write-Step "Execução limpa"
    foreach ($dialect in $Dialects.Keys) {
        Reset-Schema $dialect
        $clean = Invoke-Timed $dialect @("migrate", "run")
        $line = ($clean.Output -split "`n" | Where-Object { $_ -match "OK |ERRO" } | Select-Object -First 1)
        Write-Host ("  {0,-10} {1,7} ms  {2}" -f $dialect, $clean.Ms, $line.Trim())

        $again = Invoke-Timed $dialect @("migrate", "run")
        $idempotent = ($again.Output -match "0 aplicada").Count -gt 0

        $report += [pscustomobject]@{
            Dialeto     = $dialect
            LimpoMs     = $clean.Ms
            RerunMs     = $again.Ms
            Aplicou     = -not $clean.Failed
            Idempotente = $idempotent
        }
    }

    Write-Step "Resumo"
    $report | Format-Table -AutoSize
    $broken = @($report | Where-Object { -not $_.Aplicou -or -not $_.Idempotente })
    if ($broken.Count -gt 0) { Write-Fail "$($broken.Count) dialeto(s) com problema" }
    else { Write-Ok "4/4 aplicaram e são idempotentes" }
}

function Cmd-Help {
    Write-Host @"

GoKit testlab — ciclo de teste local

  .\testlab\lab.ps1 <comando> [dialeto] [args]

Comandos
  build                      compila e publica o .exe no projeto de teste
  reset    [dialeto|all]     zera o schema (default: all)
  run      [dialeto|all]     migrate run com tempo
  validate                   pré-validação do corpus, sem tocar no banco
  matrix                     build + validate + limpo + idempotência nos 4
  sql      <dialeto> "<q>"   query ad-hoc; aceita 'all'
  contract                   suíte de regressão do contrato de seed/ID fixo
  status                     containers e contagem de migrations aplicadas
  help

Dialetos: $($Dialects.Keys -join ', ') | all

Exemplos
  .\testlab\lab.ps1 matrix
  .\testlab\lab.ps1 reset oracle
  .\testlab\lab.ps1 sql all "SELECT id, nome FROM testando_criacao ORDER BY id"
  .\testlab\lab.ps1 contract

Projeto de teste: $TestProject
(sobrescreva com `$env:GOKIT_TESTLAB_PROJECT)

"@
}

function Cmd-Status {
    Write-Step "Containers"
    docker ps --format "  {{.Names}}`t{{.Status}}" | Where-Object { $_ -match "prevcontas_test" }

    Write-Step "Migrations aplicadas por dialeto"
    foreach ($dialect in $Dialects.Keys) {
        $count = ""
        try { $count = Invoke-SqlScalar $dialect "SELECT COUNT(*) FROM migrations_gokit" } catch { $count = "erro" }
        if (-not $count) { $count = "sem histórico" }
        Write-Host ("  {0,-10} {1}" -f $dialect, $count)
    }
}

# --------------------------------------------------------------------------

. (Join-Path $PSScriptRoot "contract.ps1")

switch ($Command.ToLower()) {
    "build" { Cmd-Build }
    "reset" { Cmd-Reset $Arg1 }
    "run" { Cmd-Run $Arg1 }
    "validate" { Cmd-Validate }
    "matrix" { Cmd-Matrix }
    "sql" { Cmd-Sql $Arg1 $Arg2 }
    "contract" { Cmd-Contract $Arg1 }
    "status" { Cmd-Status }
    default { Cmd-Help }
}
