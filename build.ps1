# Script de build multiplataforma do Go Kit

$commitHash = ""
$commitDate = ""

if (Get-Command git -ErrorAction SilentlyContinue) {
    $commitHash = (git rev-parse HEAD).Trim()
    $commitDate = (git log -n 1 --format="%ad" --date=format:"%d/%m/%Y %H:%M").Trim()
}

if (-not $commitHash) {
    $commitHash = "build-manual"
}
if (-not $commitDate) {
    $commitDate = (Get-Date -Format "dd/MM/yyyy HH:mm")
}

Write-Host "Iniciando compilação do Go Kit." -ForegroundColor Cyan
Write-Host "Commit: $commitHash" -ForegroundColor Cyan
Write-Host "Data:   $commitDate" -ForegroundColor Cyan

# Garante que a pasta de distribuição existe
$distDir = "dist"
if (-not (Test-Path $distDir)) {
    New-Item -ItemType Directory -Path $distDir | Out-Null
}

# Salva o commit atual no commit.txt
[System.IO.File]::WriteAllText("$distDir/commit.txt", $commitHash)

# Função de compilação
function Build-Target($goos, $goarch, $filename) {
    $env:GOOS = $goos
    $env:GOARCH = $goarch
    
    Write-Host "Compilando para $goos/$goarch..." -ForegroundColor Yellow
    go build -ldflags "-X main.CommitHash=$commitHash -X 'main.Version=$commitDate'" -o "$distDir/$filename" main.go
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Compilado com sucesso: $distDir/$filename" -ForegroundColor Green
    } else {
        Write-Error "Erro ao compilar para $goos/$goarch"
    }
}

# Executa builds multiplataforma
Build-Target "windows" "amd64" "gokit-windows-amd64.exe"
Build-Target "linux" "amd64" "gokit-linux-amd64"
Build-Target "darwin" "amd64" "gokit-darwin-amd64"
Build-Target "darwin" "arm64" "gokit-darwin-arm64"

# Gera o gokit.exe padrão na raiz do projeto
Write-Host "Gerando executável de desenvolvimento na raiz do repositório..." -ForegroundColor Cyan
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags "-X main.CommitHash=development -X 'main.Version=Local Development ($commitDate)'" -o "gokit.exe" main.go
if ($LASTEXITCODE -eq 0) {
    Write-Host "Sucesso: gokit.exe gerado na raiz" -ForegroundColor Green
}

# Limpa variáveis de ambiente locais
Remove-Item env:GOOS -ErrorAction SilentlyContinue
Remove-Item env:GOARCH -ErrorAction SilentlyContinue

Write-Host "Todos os binários foram gerados em ./dist/" -ForegroundColor Green
