param(
  [string]$JumpHost = "tee",
  [string]$SecondaryHost = "172.27.20.238",
  [string]$RemoteMinimalDir = "/home/ecs-user/tdid-remote-minimal"
)

$ErrorActionPreference = "Stop"
$ssh = "C:\Windows\System32\OpenSSH\ssh.exe"

function Invoke-RemoteCommand {
  param(
    [string]$Label,
    [string]$Command
  )
  $output = & $ssh $JumpHost $Command
  if ($LASTEXITCODE -ne 0) {
    throw "$Label failed with exit code $LASTEXITCODE"
  }
  return $output
}

function Invoke-RemoteHealthCheck {
  param(
    [string]$Label,
    [string]$Command
  )
  $status = Invoke-RemoteCommand -Label $Label -Command $Command
  $statusText = ($status | Out-String).Trim()
  if ($statusText -ne "200") {
    throw "$Label expected HTTP 200, got '$statusText'"
  }
  Write-Host "HTTP 200"
}

Write-Host "== 236 manifest =="
Invoke-RemoteCommand -Label "236 manifest" -Command "cat $RemoteMinimalDir/managed/current/manifest.env"

Write-Host "== 236 health:18080 =="
Invoke-RemoteHealthCheck -Label "236 health:18080" -Command "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/health"

Write-Host "== 236 health:19080 =="
Invoke-RemoteHealthCheck -Label "236 health:19080" -Command "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:19080/health"

Write-Host "== 238 manifest =="
Invoke-RemoteCommand -Label "238 manifest" -Command "ssh $SecondaryHost 'cat $RemoteMinimalDir/managed/current/manifest.env'"

Write-Host "== 238 health:18080 =="
Invoke-RemoteHealthCheck -Label "238 health:18080" -Command "ssh $SecondaryHost 'curl -sS -m 5 -o /dev/null -w ""%{http_code}"" http://127.0.0.1:18080/health'"

Write-Host "== 238 health:19080 =="
Invoke-RemoteHealthCheck -Label "238 health:19080" -Command "ssh $SecondaryHost 'curl -sS -m 5 -o /dev/null -w ""%{http_code}"" http://127.0.0.1:19080/health'"

Write-Host "== verify done =="
