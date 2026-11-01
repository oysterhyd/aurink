#!/usr/bin/env bash
# The gate (docs/05-delivery-plan.md §5.1). scripts/gate.ps1 is the same gate
# for PowerShell; change both together.
#
#   scripts/gate.sh                every check of the root module, for three targets
#   scripts/gate.sh --test-only    only the tests, on the host platform
#   scripts/gate.sh --all-targets  also windows/arm64 and darwin/amd64
#   scripts/gate.sh --spikes       also the modules under spikes/
set -euo pipefail
cd "$(dirname "$0")/.."

targets="windows/amd64 linux/amd64 darwin/arm64"
test_only=false
modules=.
for arg in "$@"; do
	case "$arg" in
	--all-targets) targets="windows/amd64 windows/arm64 linux/amd64 darwin/arm64 darwin/amd64" ;;
	--test-only) test_only=true ;;
	--spikes)
		for m in spikes/*/go.mod; do
			[ -e "$m" ] && modules="$modules $(dirname "$m")"
		done
		;;
	*)
		echo "usage: $0 [--test-only] [--all-targets] [--spikes]" >&2
		exit 2
		;;
	esac
done

export CGO_ENABLED=0
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
			GOOS=${target%/*} GOARCH=${target#*/} go -C "$d" build -o /dev/null ./...
		done
		# Production builds leave the inspector of native UI out.
		go -C "$d" vet -tags mygo_noinspector ./...
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
	check_std . ./internal/contract/... '^aurink/internal/contract(/|$)'
fi

step "5. go test"
for d in $modules; do
	echo "$d"
	go -C "$d" test -count=1 ./...
done

if ! $test_only; then
	step "6. i18ncheck"
	go run ./tools/i18ncheck
fi

echo "Gate passed"
