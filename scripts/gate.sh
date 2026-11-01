#!/usr/bin/env bash
# The merge gate (docs/05-delivery-plan.md §5.1). scripts/gate.ps1 is the same
# gate for PowerShell; change both together.
#
#   scripts/gate.sh                every check, for the three targets of §5.1
#   scripts/gate.sh --all-targets  also windows/arm64 and darwin/amd64
#   scripts/gate.sh --test-only    only the tests, on the host platform
set -euo pipefail
cd "$(dirname "$0")/.."

targets="windows/amd64 linux/amd64 darwin/arm64"
test_only=false
case "${1:-}" in
"") ;;
--all-targets) targets="windows/amd64 windows/arm64 linux/amd64 darwin/arm64 darwin/amd64" ;;
--test-only) test_only=true ;;
*)
	echo "usage: $0 [--all-targets | --test-only]" >&2
	exit 2
	;;
esac

export CGO_ENABLED=0

modules=.
for m in spikes/*/go.mod; do
	[ -e "$m" ] && modules="$modules $(dirname "$m")"
done
echo "Modules: $modules"

step() { echo "=== $*"; }

# check_std fails when packages import anything outside the standard library,
# other than the import paths matching allowed.
check_std() {
	local dir=$1 pkgs=$2 allowed=$3 bad
	bad=$(go -C "$dir" list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' "$pkgs" | grep -Ev "$allowed" || true)
	if [ -n "$bad" ]; then
		echo "$dir $pkgs imports non-standard packages:" >&2
		echo "$bad" >&2
		exit 1
	fi
}

if ! $test_only; then
	step "1. gofmt"
	out=$(gofmt -l .)
	if [ -n "$out" ]; then
		echo "Files not formatted with gofmt:" >&2
		echo "$out" >&2
		exit 1
	fi

	step "2-3. go vet and go build"
	for d in $modules; do
		for target in $targets; do
			echo "$d $target"
			GOOS=${target%/*} GOARCH=${target#*/} go -C "$d" vet ./...
			GOOS=${target%/*} GOARCH=${target#*/} go -C "$d" vet -tags mygo_noinspector ./...
			GOOS=${target%/*} GOARCH=${target#*/} go -C "$d" build -o /dev/null ./...
		done
	done

	step "4. mygo vet"
	for d in $modules; do
		# Only modules that declare the mygo tool have native UI code.
		if (cd "$d" && go tool -n mygo >/dev/null 2>&1); then
			echo "$d"
			(cd "$d" && go tool mygo vet .)
		fi
	done

	step "Contract depends on the standard library only"
	if [ -d spikes/01-contract-rc ]; then
		check_std spikes/01-contract-rc ./... '^aurink\.local/spikes/contract-rc/'
	fi
	if [ -d internal/contract ]; then
		check_std . ./internal/contract/... '^aurink/internal/contract(/|$)'
	fi
fi

step "5. go test"
for d in $modules; do
	echo "$d"
	go -C "$d" test -count=1 ./...
done

if ! $test_only; then
	step "6. i18nmerge"
	if [ -f tools/i18nmerge/main.go ]; then
		go run ./tools/i18nmerge -check
	else
		echo "SKIPPED: tools/i18nmerge does not exist yet"
	fi
fi

echo "Gate passed"
