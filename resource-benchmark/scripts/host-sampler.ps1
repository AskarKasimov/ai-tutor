<#
  Once a second, record Windows host memory/CPU and the WSL VM process (vmmemWSL).
  The Docker VM memory seen from Windows is vmmemWSL; containers are inside it, so
  host, VM and container numbers overlap and must not be added together.

  Columns: ts (Unix seconds), host_cpu_pct (all logical CPUs, 0-100),
  host_avail_mb, host_commit_mb, vmmem_ws_mb (resident in host RAM),
  vmmem_private_mb (committed by the VM process), and from nvidia-smi:
  gpu_util_pct, gpu_mem_used_mb (whole GPU, all processes), gpu_power_w, gpu_temp_c.
#>
param([Parameter(Mandatory = $true)][string]$Out)

if (-not (Test-Path $Out)) {
    'ts,host_cpu_pct,host_avail_mb,host_commit_mb,vmmem_ws_mb,vmmem_private_mb,gpu_util_pct,gpu_mem_used_mb,gpu_power_w,gpu_temp_c' | Out-File -FilePath $Out -Encoding ascii
}
while ($true) {
    $start = Get-Date
    $cpu = (Get-CimInstance Win32_PerfFormattedData_PerfOS_Processor -Filter "Name='_Total'").PercentProcessorTime
    $mem = Get-CimInstance Win32_PerfFormattedData_PerfOS_Memory
    $vm = Get-Process vmmemWSL -ErrorAction SilentlyContinue | Select-Object -First 1
    $ts = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() / 1000
    $ws = if ($vm) { [math]::Round($vm.WorkingSet64 / 1MB) } else { '' }
    $priv = if ($vm) { [math]::Round($vm.PrivateMemorySize64 / 1MB) } else { '' }
    $gpu = (& nvidia-smi --query-gpu=utilization.gpu,memory.used,power.draw,temperature.gpu --format=csv,noheader,nounits 2>$null | Select-Object -First 1) -replace ' ', ''
    if (-not $gpu) { $gpu = ',,,' }
    $line = '{0},{1},{2},{3},{4},{5},{6}' -f $ts.ToString([Globalization.CultureInfo]::InvariantCulture), $cpu, $mem.AvailableMBytes, [math]::Round($mem.CommittedBytes / 1MB), $ws, $priv, $gpu
    Add-Content -Path $Out -Value $line -Encoding ascii
    $left = 1000 - ((Get-Date) - $start).TotalMilliseconds
    if ($left -gt 0) { Start-Sleep -Milliseconds $left }
}
