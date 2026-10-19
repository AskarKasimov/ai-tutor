<#
  Snapshot of the measurement machine: hardware, OS, WSL/Docker limits, GPU and
  the repository state. Writes JSON to the given path.
#>
param([Parameter(Mandatory = $true)][string]$Out)
$root = Resolve-Path "$PSScriptRoot\..\.."
$cpu = Get-CimInstance Win32_Processor
$cs = Get-CimInstance Win32_ComputerSystem
$os = Get-CimInstance Win32_OperatingSystem
$gpu = (& nvidia-smi --query-gpu=name,memory.total,driver_version,compute_cap --format=csv,noheader) -split ',\s*'
$wsl = (& wsl --version) -join "`n" -replace "`0", ''
$info = docker info --format '{{json .}}' | ConvertFrom-Json
$dirty = git -C $root status --porcelain
$data = [ordered]@{
    captured_at        = (Get-Date).ToString('o')
    commit             = (git -C $root rev-parse HEAD)
    branch             = (git -C $root branch --show-current)
    uncommitted        = @($dirty)
    uncommitted_note   = 'routeTree.gen.ts differs only in line endings (CRLF); resource-benchmark/ is this benchmark itself; design-refs/ is unrelated and git-ignored by convention'
    cpu                = $cpu.Name.Trim()
    cpu_physical_cores = $cpu.NumberOfCores
    cpu_logical        = $cpu.NumberOfLogicalProcessors
    cpu_max_mhz        = $cpu.MaxClockSpeed
    ram_total_gib      = [math]::Round($cs.TotalPhysicalMemory / 1GB, 2)
    ram_modules        = @(Get-CimInstance Win32_PhysicalMemory | ForEach-Object { '{0} GiB {1} MT/s' -f ($_.Capacity / 1GB), $_.ConfiguredClockSpeed })
    machine            = "$($cs.Manufacturer) $($cs.Model)"
    os                 = "$($os.Caption) $($os.Version) build $($os.BuildNumber)"
    disk               = @(Get-PhysicalDisk | ForEach-Object { '{0} {1} {2:N0} GB' -f $_.FriendlyName, $_.MediaType, ($_.Size / 1GB) })
    gpu                = $gpu[0]; gpu_vram_mib = $gpu[1]; gpu_driver = $gpu[2]; gpu_compute_capability = $gpu[3]
    wsl                = $wsl
    wslconfig          = [string](Get-Content "$env:USERPROFILE\.wslconfig" -Raw)
    docker_server      = $info.ServerVersion
    docker_compose     = (docker compose version --short)
    docker_vm_cpus     = $info.NCPU
    docker_vm_mem_gib  = [math]::Round($info.MemTotal / 1GB, 2)
    docker_kernel      = $info.KernelVersion
    docker_cgroup      = $info.CgroupVersion
    docker_storage     = $info.Driver
}
$data | ConvertTo-Json -Depth 4 | Out-File -Encoding utf8 $Out
