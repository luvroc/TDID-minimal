param(
  [string]$ChainHost = "chain",
  [string]$TeeHost = "tee",
  [string]$ChainUser = "ecs-user",
  [string]$TeeUser = "ecs-user",
  [string]$ChainCloseoutScript = "/opt/tdid/p1_closeout_chain.sh",
  [string]$FiscoScript = "/opt/tdid/verify_a4_mutex_fisco.sh",
  [string]$FabricScript = "/opt/tdid/verify_a4_mutex_fabric.sh",
  [string]$TeeCloseoutScript = "/opt/tdid/p1_closeout_tee.sh",
  [string]$RunLegacyMutexSuites = "1"
)

$ErrorActionPreference = "Stop"
$ssh = "C:\Windows\System32\OpenSSH\ssh.exe"

Write-Host "== A4 matrix: chain closeout suite =="
Write-Host "== A4 matrix: note -> proof-service is treated as dev-only and not required by default =="
& $ssh "${ChainUser}@${ChainHost}" "bash $ChainCloseoutScript"
if ($LASTEXITCODE -ne 0) { throw "chain closeout suite failed" }

if ($RunLegacyMutexSuites -eq "1") {
  Write-Host "== A4 matrix (legacy): FISCO mutex negative suite =="
  & $ssh "${ChainUser}@${ChainHost}" "bash $FiscoScript"
  if ($LASTEXITCODE -ne 0) { throw "legacy FISCO mutex suite failed" }

  Write-Host "== A4 matrix (legacy): Fabric mutex negative suite =="
  & $ssh "${ChainUser}@${ChainHost}" "bash $FabricScript"
  if ($LASTEXITCODE -ne 0) { throw "legacy Fabric mutex suite failed" }
}

Write-Host "== A4 matrix: tee closeout suite =="
& $ssh "${TeeUser}@${TeeHost}" "bash $TeeCloseoutScript"
if ($LASTEXITCODE -ne 0) { throw "TEE closeout suite failed" }

Write-Host "== A4 matrix closeout: PASS =="
