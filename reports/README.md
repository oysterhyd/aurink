# reports

Each Agent appends to its own file: `int.md`, `fe-1.md`, `fe-2.md`, `be-1.md`,
`be-2.md`, `qa.md`. `quality.md` belongs to QA and `parity/` holds the Native
screenshots compared with the design baseline.

## Merge request

Add this to the end of your file when a task card is ready, then tell the user
to notify INT. INT reads it before merging (`docs/05-delivery-plan.md` §5.2).

```markdown
## Merge request: <task card ID>

- Branch: <branch>, rebased on main at <commit>
- Changes: <summary; every path must be one you own, §3.1>
- New tests and results: <tests, with key output>
- Gate: <result of scripts/gate.ps1 or gate.sh, with key output>
- INT wiring needed: <what to inject in internal/app, or "none">
- Related CR: <docs/cr/CR-*.md, or "none">
```

Mark conclusions as [实测], [源码] or [待验证]. Nothing that has not been run is
"done" or "passed".

If the gate fails on merge, INT aborts the merge and appends the failure log to
your file.
