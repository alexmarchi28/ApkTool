# Esegue i test e compila dist\ApkTool.exe (richiede Go 1.24 o successivo)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

Write-Host "1/2 Test..."
go vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet non riuscito" }
go test ./...
if ($LASTEXITCODE -ne 0) { throw "test non riusciti" }

Write-Host "2/2 Compilazione ApkTool.exe..."
New-Item -ItemType Directory -Force -Path dist | Out-Null
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
try {
    go build -trimpath -ldflags "-s -w" -o dist/ApkTool.exe .
    if ($LASTEXITCODE -ne 0) { throw "go build non riuscito" }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
Write-Host "Fatto: dist\ApkTool.exe"
