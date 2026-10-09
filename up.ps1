<#
.SYNOPSIS
    Поднимает dev-стек AI Tutor на Windows одной командой (аналог dev.sh).
.EXAMPLE
    .\up.ps1          # собрать и запустить, дождаться health, открыть браузер
    .\up.ps1 -Logs    # смотреть логи api и frontend
    .\up.ps1 -Down    # остановить; данные БД и S3 сохраняются
#>
param(
    [switch]$Down,
    [switch]$Logs
)

# Native tools report failures via exit codes, checked explicitly below.
$ErrorActionPreference = 'Continue'
Set-Location -LiteralPath $PSScriptRoot
$compose = @('compose', '--env-file', '.env', '-f', 'docker-compose.yaml')
$appUrl = 'https://localhost:8443'

function Step($text) { Write-Host "==> $text" -ForegroundColor Cyan }
function Fail($text) { Write-Host "Ошибка: $text" -ForegroundColor Red; exit 1 }
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE -ne 0) { Fail "docker $($args -join ' ') завершился с кодом $LASTEXITCODE" }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Fail 'Docker не найден. Установите Docker Desktop.'
}

if ($Down) { Invoke-Docker @compose down; exit 0 }
if ($Logs) { & docker @compose logs -f api frontend; exit 0 }

Step 'Проверяем Docker'
& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    $desktop = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    if (-not (Test-Path $desktop)) { Fail 'Docker не запущен. Запустите Docker Desktop.' }
    Write-Host '    Запускаем Docker Desktop...'
    Start-Process $desktop
    $deadline = (Get-Date).AddMinutes(3)
    do {
        Start-Sleep -Seconds 3
        & docker info *> $null
    } while ($LASTEXITCODE -ne 0 -and (Get-Date) -lt $deadline)
    if ($LASTEXITCODE -ne 0) { Fail 'Docker Desktop не запустился за 3 минуты.' }
}

if (-not (Test-Path .env)) {
    Step 'Создаём .env из .env.example (режим real, случайные секреты)'
    function New-Secret {
        -join ((48..57) + (65..90) + (97..122) | Get-Random -Count 40 | ForEach-Object { [char]$_ })
    }
    $text = (Get-Content .env.example -Raw -Encoding UTF8) -replace "`r`n", "`n"
    $text = $text -replace '(?m)^BACKEND_API_MODE=mock', 'BACKEND_API_MODE=real' `
        -replace '(?m)^VITE_API_MODE=mock', 'VITE_API_MODE=real' `
        -replace '(?m)^DB_PASSWORD=replace-with-a-long-random-value', "DB_PASSWORD=$(New-Secret)" `
        -replace '(?m)^S3_ACCESS_KEY=replace-with-a-random-access-key', "S3_ACCESS_KEY=$(New-Secret)" `
        -replace '(?m)^S3_SECRET_KEY=replace-with-a-random-secret-key', "S3_SECRET_KEY=$(New-Secret)"
    # Compose reads .env best without a BOM.
    [IO.File]::WriteAllText((Join-Path $PWD '.env'), $text, (New-Object Text.UTF8Encoding $false))
}

if (-not (Test-Path certs\server.pem) -or -not (Test-Path certs\server-key.pem)) {
    Step 'Создаём локальные HTTPS-сертификаты'
    if (-not (Get-Command mkcert -ErrorAction SilentlyContinue)) {
        Fail 'mkcert не найден. Установите: winget install FiloSottile.mkcert'
    }
    & mkcert -install
    New-Item -ItemType Directory -Force certs | Out-Null
    & mkcert -cert-file certs/server.pem -key-file certs/server-key.pem localhost 127.0.0.1 ::1
    if ($LASTEXITCODE -ne 0) { Fail 'mkcert не смог создать сертификаты.' }
}

Step 'Проверяем конфигурацию Compose'
Invoke-Docker @compose config --quiet

Step 'Собираем и запускаем контейнеры (первый запуск займёт несколько минут)'
Invoke-Docker @compose up --build -d --wait --wait-timeout 600

Step 'Проверяем API'
$health = & curl.exe -k -s -o NUL -w '%{http_code}' "$appUrl/api/v1/health"
if ($health -ne '200') {
    & docker @compose ps
    Fail "health вернул '$health'. Логи: .\up.ps1 -Logs"
}

Write-Host "Готово: $appUrl" -ForegroundColor Green
Start-Process $appUrl
