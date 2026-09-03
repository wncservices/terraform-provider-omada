<!--
Thanks for the contribution. See CONTRIBUTING.md for the full workflow —
this template mirrors its checklist so the essentials aren't missed.
-->

## What & why

<!-- What does this change, and what problem does it solve? Link the issue it closes, if any. -->

Closes #

## How was this verified?

<!--
Unit + acceptance tests run against the in-process mock controller and need
no hardware (`make test`, `TF_ACC=1 make testacc`). If this touches a real
API verb, payload shape, or error code, say how it was confirmed against a
live controller (model + firmware version) — see DESIGN.md §1 on why nothing
here is assumed.
-->

## Checklist

- [ ] `make build && make test && TF_ACC=1 make testacc && make lint` all pass locally
- [ ] New/changed endpoints have mock coverage in `newMockController` (`internal/provider/provider_test.go`) and an acceptance test that does create → import (`ImportStateVerify`) → update
- [ ] Schema `MarkdownDescription`s updated and `make docs` run — `docs/` is generated, never hand-edited
- [ ] `examples/resources/omada_<name>/resource.tf` (and `import.sh` if it imports) added or updated, if applicable
- [ ] No controller credentials, secrets, or `*.tfstate` in the diff
