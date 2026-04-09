$ErrorActionPreference = "Stop"

$ssh = "C:\Windows\System32\OpenSSH\ssh.exe"
$scp = "C:\Windows\System32\OpenSSH\scp.exe"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$ChainScript = Join-Path $RepoRoot "scripts\closeout\p1_closeout_chain.sh"
$TeeScript = Join-Path $RepoRoot "scripts\closeout\p1_closeout_tee.sh"

Write-Host "== P1 Closeout: sync scripts =="
& $scp $ChainScript "chain:/home/ecs-user/p1_closeout_chain.sh"
& $scp $TeeScript "tee:/home/ecs-user/p1_closeout_tee.sh"

& $ssh "chain" "python3 - <<'PY'
from pathlib import Path
for p in (Path('/home/ecs-user/p1_closeout_chain.sh'),
          Path('/home/ecs-user/p15_fabric_withproof_probe.sh'),
          Path('/home/ecs-user/p15_fabric_withproof_call_probe.sh'),
          Path('/home/ecs-user/p15_fabric_withproof_positive_dualcc.sh'),
          Path('/home/ecs-user/a4_deploy_and_test_agent123_plus.sh')):
    if p.exists():
        b=p.read_bytes()
        if b.startswith(b'\xef\xbb\xbf'):
            b=b[3:]
        b=b.replace(b'\r\n', b'\n')
        p.write_bytes(b)
print('chain normalized')
PY
chmod +x /home/ecs-user/p1_closeout_chain.sh /home/ecs-user/p15_fabric_withproof_probe.sh /home/ecs-user/p15_fabric_withproof_call_probe.sh /home/ecs-user/p15_fabric_withproof_positive_dualcc.sh /home/ecs-user/a4_deploy_and_test_agent123_plus.sh"

& $ssh "tee" "python3 - <<'PY'
from pathlib import Path
p=Path('/home/ecs-user/p1_closeout_tee.sh')
if p.exists():
    b=p.read_bytes()
    if b.startswith(b'\xef\xbb\xbf'):
        b=b[3:]
    b=b.replace(b'\r\n', b'\n')
    p.write_bytes(b)
print('tee normalized')
PY
chmod +x /home/ecs-user/p1_closeout_tee.sh"

Write-Host "== P1 Closeout: run chain suite =="
& $ssh "chain" "bash /home/ecs-user/p1_closeout_chain.sh"

Write-Host "== P1 Closeout: run tee suite =="
& $ssh "tee" "bash /home/ecs-user/p1_closeout_tee.sh"

Write-Host "== P1 Closeout: ALL PASS =="
