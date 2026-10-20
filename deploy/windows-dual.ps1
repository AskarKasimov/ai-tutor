# Runs two releases side by side behind one HTTPS entry point (Windows):
#   https://$DUAL_HOST        start page with one login for both
#   https://learn.$DUAL_HOST  learning (DUAL_LEARN_REF, v0.0.2.5)
#   https://exam.$DUAL_HOST   exam simulation (DUAL_EXAM_REF, latest master)
# Each release runs from its own git worktree with its own .env, volumes and
# network. Linux and macOS use linux-dual.sh. See dual/README.md.

# Native tools report progress on stderr; failures are checked by exit code.
$ErrorActionPreference = 'Continue'
Set-StrictMode -Version 2

$dualDir = Join-Path $PSScriptRoot 'dual'
$repoDir = (Resolve-Path (Join-Path $dualDir '..\..')).Path

function Get-Setting([string]$name, [string]$default) {
    $value = [Environment]::GetEnvironmentVariable($name)
    if ([string]::IsNullOrEmpty($value)) { return $default }
    return $value
}

$DualHost = Get-Setting 'DUAL_HOST' '127-0-0-1.sslip.io'
$HttpsBind = Get-Setting 'DUAL_HTTPS_BIND' '127.0.0.1'
$HttpsPort = Get-Setting 'DUAL_HTTPS_PORT' '443'
$Mode = Get-Setting 'DUAL_MODE' 'prod'
$LearnRef = Get-Setting 'DUAL_LEARN_REF' 'v0.0.2.5'
$ExamRef = Get-Setting 'DUAL_EXAM_REF' 'master'
$Worktrees = Get-Setting 'DUAL_WORKTREES' (Split-Path $repoDir)
$OpenBrowser = Get-Setting 'DUAL_OPEN_BROWSER' 'true'

function Show-Usage {
    [Console]::Error.WriteLine(@'
Usage: .\deploy\windows-dual.ps1 up|down|status|logs [learn|exam|front]

Environment, set with $env:NAME = 'value' (defaults in brackets):
  DUAL_HOST        base host name [127-0-0-1.sslip.io]; on a VM: <ip-with-dashes>.sslip.io
  DUAL_HTTPS_BIND  address to publish HTTPS on [127.0.0.1]; on a VM: 0.0.0.0
  DUAL_HTTPS_PORT  HTTPS port [443]
  DUAL_MODE        prod (built frontend and binary) or dev (Vite and Air) [prod]
  DUAL_LEARN_REF   git ref of the learning release [v0.0.2.5]
  DUAL_EXAM_REF    git ref of the exam simulation: a tag before the show [master]
  DUAL_WORKTREES   directory for the release worktrees [parent of the repository]
  DUAL_OPEN_BROWSER=false skips opening the browser
'@)
    exit 2
}

function Fail([string]$message) {
    [Console]::Error.WriteLine($message)
    exit 1
}

if ($args.Count -lt 1) { Show-Usage }
$command = $args[0]
$rest = @($args | Select-Object -Skip 1)

if ($Mode -ne 'prod' -and $Mode -ne 'dev') { [Console]::Error.WriteLine("DUAL_MODE must be prod or dev, got: $Mode"); exit 2 }

if ($HttpsPort -eq '443') { $originHost = $DualHost } else { $originHost = "${DualHost}:$HttpsPort" }
$DualOrigin = "https://$originHost"
$env:DUAL_HOST = $DualHost
$env:DUAL_HTTPS_BIND = $HttpsBind
$env:DUAL_HTTPS_PORT = $HttpsPort
$env:DUAL_ORIGIN = $DualOrigin
# up picks the certificate in Initialize-FrontTls; other commands only need a value.
$env:DUAL_TLS = 'internal'

# Get-Release learn|exam: ref, tree, project and published ports.
function Get-Release([string]$name) {
    switch ($name) {
        'learn' { $r = @{ Ref = $LearnRef; Ports = 5442, 8012, 9010, 8444; Profiles = '' } }
        'exam' { $r = @{ Ref = $ExamRef; Ports = 5443, 8013, 9011, 8445; Profiles = 'gateway' } }
    }
    $r.Name = $name
    $r.Tree = Join-Path $Worktrees "ai-tutor-$name"
    $r.Project = "tutor-$name"
    $r.DbPort, $r.ApiPort, $r.S3Port, $r.ProxyPort = $r.Ports
    return $r
}

function Invoke-Native([string]$exe, [string[]]$arguments) {
    & $exe @arguments
    if ($LASTEXITCODE -ne 0) { Fail "$exe $($arguments -join ' ') failed with exit code $LASTEXITCODE." }
}

function Invoke-ReleaseCompose($r, [string[]]$arguments) {
    Invoke-Native docker (@('compose', '-p', $r.Project, '--project-directory', $r.Tree,
            '--env-file', "$($r.Tree)\.env", '-f', "$($r.Tree)\docker-compose.yaml",
            '-f', "$($r.Tree)\compose.dual.yaml") + $arguments)
}

function Invoke-FrontCompose([string[]]$arguments) {
    Invoke-Native docker (@('compose', '-p', 'tutor-dual', '-f', "$dualDir\docker-compose.yaml") + $arguments)
}

# UTF-8 without BOM and LF, as Compose and the release containers expect.
function Write-TextFile([string]$path, [string[]]$lines) {
    [IO.File]::WriteAllText($path, (($lines -join "`n") + "`n"), (New-Object Text.UTF8Encoding $false))
}

# Set-EnvValue FILE KEY VALUE replaces or appends KEY=VALUE.
function Set-EnvValue([string]$path, [string]$key, [string]$value) {
    $found = $false
    $lines = @(foreach ($line in [IO.File]::ReadAllLines($path)) {
            if ($line.StartsWith("$key=")) { $found = $true; "$key=$value" } else { $line }
        })
    if (-not $found) { $lines += "$key=$value" }
    Write-TextFile $path $lines
}

function Get-RandomHex([int]$bytes) {
    $buffer = New-Object byte[] $bytes
    [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($buffer)
    return -join ($buffer | ForEach-Object { $_.ToString('x2') })
}

# openssl from PATH or from Git for Windows.
function Find-OpenSsl {
    $cmd = Get-Command openssl -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $git = Get-Command git -ErrorAction SilentlyContinue
    if ($git) {
        $gitRoot = Split-Path (Split-Path $git.Source)
        foreach ($candidate in "$gitRoot\usr\bin\openssl.exe", "$gitRoot\mingw64\bin\openssl.exe") {
            if (Test-Path $candidate) { return $candidate }
        }
    }
    return $null
}

function Initialize-Worktree([string]$name) {
    $r = Get-Release $name
    $tree = $r.Tree
    # A branch name follows origin (master is the latest master on every up),
    # a tag stays where it is.
    & git -C $repoDir fetch --tags --quiet origin
    if ($LASTEXITCODE -ne 0) { [Console]::Error.WriteLine('Warning: git fetch failed; using local refs.') }
    & git -C $repoDir rev-parse --verify --quiet "refs/remotes/origin/$($r.Ref)" *> $null
    if ($LASTEXITCODE -eq 0) {
        $want = & git -C $repoDir rev-parse "refs/remotes/origin/$($r.Ref)^{commit}"
    } else {
        $want = & git -C $repoDir rev-parse --verify "$($r.Ref)^{commit}"
        if ($LASTEXITCODE -ne 0) { Fail "Unknown git ref: $($r.Ref)" }
    }
    if (-not (Test-Path $tree)) {
        Invoke-Native git @('-C', $repoDir, 'worktree', 'add', '--detach', $tree, $want)
    }
    $have = & git -C $tree rev-parse HEAD
    if ($want -ne $have) {
        if (& git -C $tree status --porcelain --untracked-files=no) {
            Fail "$tree has uncommitted changes; commit or discard them to switch to $($r.Ref)."
        }
        Invoke-Native git @('-C', $tree, 'switch', '--quiet', '--detach', $want)
        Write-Output "Switched $tree to $($r.Ref) ($(& git -C $tree log --oneline -1))."
        Write-Output "New settings of $($r.Ref) go to $tree\.env by hand; compare it with .env.example."
    }

    $envFile = "$tree\.env"
    if (-not (Test-Path $envFile)) {
        Copy-Item "$tree\.env.example" $envFile
        Set-EnvValue $envFile DB_PASSWORD (Get-RandomHex 16)
        Set-EnvValue $envFile S3_ACCESS_KEY (Get-RandomHex 10)
        Set-EnvValue $envFile S3_SECRET_KEY (Get-RandomHex 20)
        # Voice settings follow the main checkout when it has its own .env.
        if (Test-Path "$repoDir\.env") {
            foreach ($line in [IO.File]::ReadAllLines("$repoDir\.env")) {
                if ($line -cmatch '^(BACKEND_(STT|TTS)_[A-Z_]+)=(.*)$') {
                    Set-EnvValue $envFile $Matches[1] $Matches[3]
                }
            }
        }
        Write-Output "Created $envFile from its template; review it if models or voice differ."
    }
    Set-EnvValue $envFile DB_PORT $r.DbPort
    Set-EnvValue $envFile S3_PORT $r.S3Port
    Set-EnvValue $envFile PROXY_HTTPS_BIND 127.0.0.1
    Set-EnvValue $envFile PROXY_HTTPS_PORT $r.ProxyPort
    Set-EnvValue $envFile COMPOSE_PROFILES $r.Profiles
    # Mode choice in the profile menu and logout through the start page.
    Set-EnvValue $envFile VITE_MODE_SELECT_URL $DualOrigin

    # The gateway's models and tokens are not in git: the release takes them
    # from the main checkout. Without them every model call fails, so stop;
    # a copy of the example left by an earlier run is replaced.
    $gatewayDir = "$tree\services\llm-gateway"
    if (Test-Path "$gatewayDir\config.example.json") {
        if ((Test-Path "$gatewayDir\config.json") -and
            ((Get-FileHash "$gatewayDir\config.json").Hash -eq (Get-FileHash "$gatewayDir\config.example.json").Hash)) {
            Remove-Item -Force "$gatewayDir\config.json"
        }
        if (-not (Test-Path "$gatewayDir\config.json") -and (Test-Path "$repoDir\services\llm-gateway\config.json")) {
            Copy-Item "$repoDir\services\llm-gateway\config.json" "$gatewayDir\config.json"
        }
        if (-not (Test-Path "$gatewayDir\config.json") -or
            ((Get-FileHash "$gatewayDir\config.json").Hash -eq (Get-FileHash "$gatewayDir\config.example.json").Hash)) {
            Remove-Item -Force -ErrorAction SilentlyContinue "$gatewayDir\config.json"
            Fail "No gateway models: put config.json with the models and their tokens into $repoDir\services\llm-gateway\config.json and run up again."
        }
    }

    # The release's own Caddy only talks to the front proxy; any certificate will do.
    if (-not (Test-Path "$tree\certs\server.pem")) {
        New-Item -ItemType Directory -Force "$tree\certs" | Out-Null
        $openssl = Find-OpenSsl
        if (-not $openssl) { Fail 'openssl is required: install Git for Windows or put openssl on PATH.' }
        & $openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj /CN=localhost `
            -addext subjectAltName=DNS:localhost `
            -keyout "$tree\certs\server-key.pem" -out "$tree\certs\server.pem" *> $null
        if ($LASTEXITCODE -ne 0) { Fail "openssl could not create $tree\certs\server.pem." }
    }

    Write-Override $r
}

# The releases hard-code the names ai-tutor_default and ai-tutor_*; the
# override gives each release its own network, volumes and host ports.
function Write-Override($r) {
    $project = $r.Project
    $volumes = 'postgres_data', 'task_audio_data', 'caddy_data', 'caddy_config'
    # Compose rejects names for volumes the release does not declare.
    if (Select-String -Path "$($r.Tree)\docker-compose.yaml" -Pattern 'redpanda:' -SimpleMatch -Quiet) {
        $volumes += 'redpanda_data'
    }
    $lines = @('# Generated by deploy/windows-dual.ps1; edits are overwritten.',
        'networks:', '  default:', "    name: $project", 'volumes:')
    foreach ($volume in $volumes) { $lines += "  ${volume}:", "    name: ${project}_$volume" }
    $lines += 'services:', '  api:', '    ports: !override', "      - `"127.0.0.1:$($r.ApiPort):8002`""
    if ($Mode -eq 'prod') {
        # Final Dockerfile stage, no source mounts: the built binary.
        $lines += '    build:', '      target: !reset null'
        if (Test-Path "$($r.Tree)\api\fixtures") {
            $lines += '    volumes: !override', '      - ./api/fixtures:/fixtures:ro'
        } else {
            $lines += '    volumes: !reset []'
        }
        $lines += '    healthcheck:', '      test: ["CMD", "/api", "healthcheck"]', '      start_period: 60s'
        $lines += '  frontend:', '    build:', '      target: !reset null', '    volumes: !reset []'
    }
    $lines += '  s3:', '    ports: !override', "      - `"127.0.0.1:$($r.S3Port):9000`""
    Write-TextFile "$($r.Tree)\compose.dual.yaml" $lines
}

function Initialize-FrontTls {
    New-Item -ItemType Directory -Force "$dualDir\certs" | Out-Null
    if (Get-Command mkcert -ErrorAction SilentlyContinue) {
        & mkcert -install *> $null
        Invoke-Native mkcert @('-cert-file', "$dualDir\certs\dual.pem", '-key-file', "$dualDir\certs\dual-key.pem",
            $DualHost, "learn.$DualHost", "exam.$DualHost") *> $null
        $env:DUAL_TLS = '/certs/dual.pem /certs/dual-key.pem'
    } else {
        # Caddy's own CA: the browser warns once per host.
        $env:DUAL_TLS = 'internal'
    }
}

function Test-Dns {
    foreach ($name in $DualHost, "learn.$DualHost", "exam.$DualHost") {
        if (-not (Resolve-DnsName $name -ErrorAction SilentlyContinue)) {
            [Console]::Error.WriteLine(@"
Warning: $name does not resolve (no internet or DNS rebinding protection).
Add it to C:\Windows\System32\drivers\etc\hosts with the machine's address, for example:
  127.0.0.1 $DualHost learn.$DualHost exam.$DualHost
"@)
            return
        }
    }
}

function Open-Browser {
    if ($OpenBrowser -eq 'false') { return }
    foreach ($url in "https://learn.$originHost/admin/competency-map", "https://exam.$originHost/admin", $DualOrigin) {
        try { Start-Process $url } catch { }
    }
}

switch ($command) {
    'up' {
        if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { Fail 'Docker with Compose is required.' }
        foreach ($name in 'learn', 'exam') {
            Initialize-Worktree $name
            Invoke-ReleaseCompose (Get-Release $name) @('config', '--quiet')
        }
        Initialize-FrontTls
        Invoke-FrontCompose @('config', '--quiet')
        Test-Dns
        foreach ($name in 'learn', 'exam') {
            $r = Get-Release $name
            Write-Output "Starting $name ($($r.Ref), $Mode); the first build takes several minutes."
            Invoke-ReleaseCompose $r (@('up', '--build', '--wait', '--detach') + $rest)
        }
        Invoke-FrontCompose @('up', '--wait', '--detach', '--force-recreate')
        Write-Output "Start page:       $DualOrigin"
        Write-Output "Learning:         https://learn.$originHost  (admin: /admin/competency-map)"
        Write-Output "Exam simulation:  https://exam.$originHost  (admin: /admin)"
        Open-Browser
    }
    'down' {
        Invoke-FrontCompose @('down')
        foreach ($name in 'exam', 'learn') {
            $r = Get-Release $name
            if (Test-Path "$($r.Tree)\compose.dual.yaml") { Invoke-ReleaseCompose $r @('down') }
        }
    }
    'status' {
        Invoke-FrontCompose @('ps')
        foreach ($name in 'learn', 'exam') {
            $r = Get-Release $name
            if (Test-Path "$($r.Tree)\compose.dual.yaml") { Invoke-ReleaseCompose $r @('ps') }
        }
    }
    'logs' {
        $target = if ($rest.Count -ge 1) { $rest[0] } else { '' }
        $more = @($rest | Select-Object -Skip 1)
        switch ($target) {
            { $_ -in 'learn', 'exam' } { Invoke-ReleaseCompose (Get-Release $target) (@('logs', '--tail', '200', '-f') + $more) }
            'front' { Invoke-FrontCompose @('logs', '--tail', '200', '-f') }
            default { Show-Usage }
        }
    }
    default { Show-Usage }
}
