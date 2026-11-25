[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Kubeconfig,
    [Parameter(Mandatory = $true)][string]$Context,
    [string]$K3dExecutable
)

$ErrorActionPreference = 'Stop'
$cluster = 'loomspan-cloud-pr1-cg1'
if ($Context -ne "k3d-$cluster") {
    throw 'The milestone creates disposable state and replaces one engine pod. Use its dedicated context.'
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
$kube = @('--kubeconfig', (Resolve-Path -LiteralPath $Kubeconfig).Path, '--context', $Context)
$engineNamespace = 'loomspan-engine'
$managementNamespace = 'loomspan-management'
$engineDeployment = 'deployment/loomspan-execution-engine'
$managementDeployment = 'deployment/loomspan-management-backend'
$engineSelector = 'app.kubernetes.io/name=loomspan-execution-engine'
$probeJob = 'job/loomspan-restored-skill'

if (-not $K3dExecutable) {
    $K3dExecutable = Join-Path $repo 'execution-engine/target/pr1-k3s/k3d.exe'
    if (-not (Test-Path -LiteralPath $K3dExecutable)) {
        $K3dExecutable = (Get-Command k3d).Source
    }
}

function Invoke-Kube {
    & kubectl @kube @args
    if ($LASTEXITCODE -ne 0) { throw "kubectl failed: $($args[0])" }
}

function Invoke-BuildTool([string]$ToolName, [string[]]$Arguments) {
    & $ToolName @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Build command failed: $ToolName" }
}

$serverAddress = Invoke-Kube config view --minify -o 'jsonpath={.clusters[0].cluster.server}'
if (-not ([uri]$serverAddress).IsLoopback) {
    throw 'The selected disposable cluster must have a loopback API server.'
}
$nodes = @(Invoke-Kube get nodes -o name)
if ($nodes.Count -ne 1 -or $nodes[0] -ne "node/k3d-$cluster-server-0") {
    throw 'The selected API server does not identify the dedicated single-node k3d cluster.'
}
Invoke-Kube get nodes
Invoke-Kube -n $engineNamespace get $engineDeployment
Invoke-Kube -n $managementNamespace get $managementDeployment
Invoke-Kube -n $engineNamespace wait --for=condition=Ready certificate/engine-server --timeout=60s
Invoke-Kube -n $managementNamespace wait --for=condition=Ready certificate/management-client --timeout=60s

$stagingRoot = Join-Path $repo ('execution-engine/target/pr15-k3s-' + [guid]::NewGuid().ToString('N'))
$engineBuild = Join-Path $stagingRoot 'engine'
$probeBuild = Join-Path $stagingRoot 'probe'
$fixturePackage = 'ai/loomspan/cloud/engine/fixture'
$fixtureDest = Join-Path $engineBuild "fixture/$fixturePackage"
$migrationDest = Join-Path $probeBuild 'migrations'
New-Item -ItemType Directory -Path $fixtureDest, $migrationDest -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $repo 'execution-engine/target/loomspan-cloud-execution-engine-0.1.0-SNAPSHOT.jar') -Destination (Join-Path $engineBuild 'engine.jar')
Get-ChildItem -LiteralPath (Join-Path $repo "execution-engine/target/test-classes/$fixturePackage") -Filter 'MilestoneExecutorConfiguration*.class' |
    Copy-Item -Destination $fixtureDest
if (-not (Test-Path -LiteralPath (Join-Path $fixtureDest 'MilestoneExecutorConfiguration.class'))) {
    throw 'Run Maven verify before staging the engine and test fixture classes.'
}
foreach ($migration in @('001_publications.sql', '002_invocation.sql')) {
    Copy-Item -LiteralPath (Join-Path $repo "management-backend/migrations/$migration") -Destination $migrationDest
}

$savedGoOS = $env:GOOS
$savedGoArch = $env:GOARCH
$savedCgo = $env:CGO_ENABLED
Push-Location (Join-Path $repo 'management-backend')
try {
    $arch = Invoke-Kube get nodes -o 'jsonpath={.items[0].status.nodeInfo.architecture}'
    $env:GOOS = 'linux'
    $env:GOARCH = $arch
    $env:CGO_ENABLED = '0'
    Invoke-BuildTool 'go' @('test', '-c', '-tags=k3s_integration', '-o', (Join-Path $probeBuild 'milestone-probe'), './internal/invocation')
} finally {
    Pop-Location
    $env:GOOS = $savedGoOS
    $env:GOARCH = $savedGoArch
    $env:CGO_ENABLED = $savedCgo
}

$engineImage = 'loomspan-milestone-engine:local'
$probeImage = 'loomspan-milestone-probe:local'
Invoke-BuildTool 'docker' @('build', '-t', $engineImage, '-f', (Join-Path $PSScriptRoot 'execution-engine.Dockerfile'), $engineBuild)
Invoke-BuildTool 'docker' @('build', '-t', $probeImage, '-f', (Join-Path $PSScriptRoot 'probe.Dockerfile'), $probeBuild)
Invoke-BuildTool $K3dExecutable @('image', 'import', $engineImage, $probeImage, 'postgres:17-alpine', '--cluster', $cluster)

# The probe is the sole Go writer and watches the same engine selector as the deployment.
Invoke-Kube -n $managementNamespace scale $managementDeployment --replicas=0
Invoke-Kube -n $engineNamespace scale $engineDeployment --replicas=0
$waitForEngines = [DateTime]::UtcNow.AddSeconds(100)
while (Invoke-Kube -n $engineNamespace get pods -l $engineSelector -o name) {
    if ([DateTime]::UtcNow -ge $waitForEngines) { throw 'Previous engine pod did not terminate.' }
    Start-Sleep -Milliseconds 250
}
Invoke-Kube -n $managementNamespace delete $probeJob --ignore-not-found --cascade=foreground --timeout=120s
Invoke-Kube -n $managementNamespace delete deployment/loomspan-milestone-postgres --ignore-not-found --cascade=foreground --timeout=120s
Invoke-Kube -n $managementNamespace delete pvc/loomspan-milestone-db pvc/loomspan-milestone-packs --ignore-not-found --timeout=120s

$keyFile = Join-Path ([System.IO.Path]::GetTempPath()) ('loomspan-milestone-' + [guid]::NewGuid().ToString('N'))
try {
    [System.IO.File]::WriteAllText($keyFile, [Convert]::ToHexString([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(24)))
    $secret = & kubectl @kube -n $managementNamespace create secret generic loomspan-milestone-db "--from-file=password=$keyFile" --dry-run=client -o yaml
    if ($LASTEXITCODE -ne 0) { throw 'Failed to generate disposable database Secret.' }
    $secret | & kubectl @kube -n $managementNamespace apply -f -
    if ($LASTEXITCODE -ne 0) { throw 'Failed to apply disposable database Secret.' }
} finally {
    if (Test-Path -LiteralPath $keyFile) { Remove-Item -LiteralPath $keyFile -Force }
}

Invoke-Kube apply -f (Join-Path $PSScriptRoot 'deployment.yaml')
Invoke-Kube -n $managementNamespace rollout status deployment/loomspan-milestone-postgres --timeout=90s
$enginePatch = Join-Path $stagingRoot 'engine-patch.json'
@{spec = @{template = @{spec = @{containers = @(@{
    name = 'execution-engine'
    image = $engineImage
    imagePullPolicy = 'Never'
    env = @(@{name = 'SPRING_PROFILES_ACTIVE'; value = 'k3s,milestone-fixture'})
})}}}} | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $enginePatch -Encoding utf8
Invoke-Kube -n $engineNamespace patch $engineDeployment --type=strategic --patch-file $enginePatch
Invoke-Kube apply -f (Join-Path $PSScriptRoot 'job.yaml')
Invoke-Kube -n $engineNamespace scale $engineDeployment --replicas=1

$checkpoint = $null
$checkpointDeadline = [DateTime]::UtcNow.AddSeconds(155)
while (-not $checkpoint -and [DateTime]::UtcNow -lt $checkpointDeadline) {
    $output = & kubectl @kube -n $managementNamespace logs $probeJob 2>$null
    $checkpoint = $output | Select-String '^K3S_MILESTONE_REPLACE (\S+) (\S+) (\S+) (\S+)$' | Select-Object -First 1
    if (-not $checkpoint) {
        $failed = Invoke-Kube -n $managementNamespace get $probeJob -o 'jsonpath={.status.failed}'
        if ($failed -eq '1') { break }
        Start-Sleep -Milliseconds 400
    }
}
if (-not $checkpoint) {
    Invoke-Kube -n $managementNamespace logs $probeJob
    throw 'The probe did not reach its durable acknowledgment checkpoint.'
}
$oldPod = $checkpoint.Matches[0].Groups[1].Value
$oldUID = $checkpoint.Matches[0].Groups[2].Value
$liveUID = Invoke-Kube -n $engineNamespace get "pod/$oldPod" -o 'jsonpath={.metadata.uid}'
$enginePods = @(Invoke-Kube -n $engineNamespace get pods -l $engineSelector -o name)
if ($oldUID -ne $liveUID -or $enginePods.Count -ne 1 -or $enginePods[0] -ne "pod/$oldPod") {
    throw 'The checkpoint does not identify the sole live fixture engine pod.'
}
$checkpoint.Line
# Scaling to zero terminates the verified sole pod and holds its ReplicaSet at
# zero until it is gone. A direct delete could create its replacement early.
Invoke-Kube -n $engineNamespace scale $engineDeployment --current-replicas=1 --replicas=0
$retirementDeadline = [DateTime]::UtcNow.AddSeconds(85)
while (Invoke-Kube -n $engineNamespace get "pod/$oldPod" --ignore-not-found -o name) {
    if ([DateTime]::UtcNow -ge $retirementDeadline) { throw 'The checkpointed engine pod did not terminate.' }
    Start-Sleep -Milliseconds 250
}
Invoke-Kube -n $engineNamespace scale $engineDeployment --current-replicas=0 --replicas=1

$finishedDeadline = [DateTime]::UtcNow.AddSeconds(160)
while ([DateTime]::UtcNow -lt $finishedDeadline) {
    $succeeded = Invoke-Kube -n $managementNamespace get $probeJob -o 'jsonpath={.status.succeeded}'
    if ($succeeded -eq '1') {
        Invoke-Kube -n $managementNamespace logs $probeJob
        return
    }
    $failed = Invoke-Kube -n $managementNamespace get $probeJob -o 'jsonpath={.status.failed}'
    if ($failed -eq '1') { break }
    Start-Sleep -Milliseconds 400
}
Invoke-Kube -n $managementNamespace logs $probeJob
throw 'The scoped recovery probe did not pass.'
