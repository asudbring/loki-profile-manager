$ErrorActionPreference = "Stop"

Write-Host "== go env =="
go env GOOS GOARCH GOVERSION

Write-Host "== go test =="
go test ./...

Write-Host "== go vet =="
go vet ./...

Write-Host "== go mod verify =="
go mod verify

Write-Host "== go build =="
New-Item -ItemType Directory -Force .\bin | Out-Null
$exe = if ((go env GOOS) -eq "windows") { ".\bin\loki.exe" } else { ".\bin\loki" }
go build -o $exe .\cmd\loki

$goos = go env GOOS
$goarch = go env GOARCH
if ($goos -eq "windows" -and $goarch -eq "arm64") {
  Write-Host "== go test -race =="
  Write-Host "skipped: Go race detector is not supported on windows/arm64"
} else {
  Write-Host "== go test -race =="
  go test -race ./...
}

Write-Host "validation complete"
