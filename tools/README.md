# tools

Owner: INT.

- `i18nmerge` checks `i18n/fragments` against the key rules of
  `docs/05-delivery-plan.md` §3.4 and generates `locales`. Run it from the
  repository root after changing a fragment: `go run ./tools/i18nmerge`. The
  gate runs it with `-check`.
- `gate` is reserved. The gate is `scripts/gate.ps1` and `scripts/gate.sh`.
