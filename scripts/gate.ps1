# The merge gate (docs/05-delivery-plan.md §5.1). scripts/gate.sh is the same
# gate for bash; change both together.
#
#   scripts/gate.ps1               every check, for the three targets of §5.1
#   scripts/gate.ps1 -AllTargets   also windows/arm64 and darwin/amd64
#   scripts/gate.ps1 -TestOnly     only the tests, on the host platform
[CmdletBinding()]
param(
    [switch]$AllTargets,
    [switch]$TestOnly
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-Location (Split-Path -Parent $PSScriptRoot)

$targets = 'windows/amd64', 'linux/amd64', 'darwin/arm64'
if ($AllTargets) {
    $targets = 'windows/amd64', 'windows/arm64', 'linux/amd64', 'darwin/arm64', 'darwin/amd64'
}

$env:CGO_ENABLED = '0'

$modules = @('.')
foreach ($m in Get-ChildItem -Path spikes -Filter go.mod -Recurse -Depth 1 -ErrorAction SilentlyContinue) {
    $modules += "spikes/$($m.Directory.Name)"
}
Write-Host "Modules: $modules"

function Step($name) { Write-Host "=== $name" }

# Run runs a native command and stops the gate when it fails.
function Run {
    & $args[0] @($args | Select-Object -Skip 1)
    if ($LASTEXITCODE -ne 0) { throw "failed ($LASTEXITCODE): $args" }
}

# CheckStd fails when packages import anything outside the standard library,
# other than the import paths matching allowed.
function CheckStd($dir, $pkgs, $allowed) {
    $deps = go -C $dir list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' $pkgs
    if ($LASTEXITCODE -ne 0) { throw "go list failed in $dir" }
    $bad = @($deps | Where-Object { $_ -and $_ -notmatch $allowed })
    if ($bad.Count -gt 0) { throw "$dir $pkgs imports non-standard packages:`n$($bad -join "`n")" }
}

$savedGOOS, $savedGOARCH = $env:GOOS, $env:GOARCH
try {
    if (-not $TestOnly) {
        Step '1. gofmt'
        $out = gofmt -l .
        if ($LASTEXITCODE -ne 0) { throw 'gofmt failed' }
        if ($out) { throw "Files not formatted with gofmt:`n$($out -join "`n")" }

        Step '2-3. go vet and go build'
        foreach ($d in $modules) {
            foreach ($target in $targets) {
                Write-Host "$d $target"
                $env:GOOS, $env:GOARCH = $target -split '/'
                Run go -C $d vet ./...
                Run go -C $d vet -tags mygo_noinspector ./...
                Run go -C $d build -o NUL ./...
            }
        }
        $env:GOOS, $env:GOARCH = $savedGOOS, $savedGOARCH

        Step '4. mygo vet'
        foreach ($d in $modules) {
            # Only modules that declare the mygo tool have native UI code.
            Push-Location $d
            try {
                go tool -n mygo *> $null
                if ($LASTEXITCODE -eq 0) {
                    Write-Host $d
                    Run go tool mygo vet .
                }
            }
            finally { Pop-Location }
        }

        Step 'Contract depends on the standard library only'
        if (Test-Path spikes/01-contract-rc) {
            CheckStd spikes/01-contract-rc ./... '^aurink\.local/spikes/contract-rc/'
        }
        if (Test-Path internal/contract) {
            CheckStd . ./internal/contract/... '^aurink/internal/contract(/|$)'
        }
    }

    Step '5. go test'
    foreach ($d in $modules) {
        Write-Host $d
        Run go -C $d test -count=1 ./...
    }

    if (-not $TestOnly) {
        Step '6. i18nmerge'
        if (Test-Path tools/i18nmerge/main.go) {
            Run go run ./tools/i18nmerge -check
        }
        else {
            Write-Host 'SKIPPED: tools/i18nmerge does not exist yet'
        }
    }
}
finally {
    $env:GOOS, $env:GOARCH = $savedGOOS, $savedGOARCH
}

Write-Host 'Gate passed'
