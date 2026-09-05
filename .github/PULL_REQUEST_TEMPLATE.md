## What this changes

<!-- One paragraph. Name the resource or the area, and the Dokploy behavior that drives the change. -->

## Checklist for a resource or data source change

- [ ] `internal/client/<name>.go` and its test: the endpoints and the wire shape
- [ ] `internal/client/dialect_test.go`: every new request struct
- [ ] `internal/resources/<name>/`: schema, CRUD, `ImportState`, and the model tests
- [ ] `*_acc_test.go`: a step that drops each optional attribute and asserts an empty plan; import with `ImportStateVerify`
- [ ] `internal/provider/provider.go`: the registration
- [ ] `examples/resources/dokploy_<name>/`: `resource.tf` and `import.sh`
- [ ] `make docs`: the generated page is committed
- [ ] `templates/index.md.tmpl`, `README.md`, and `CHANGELOG.md`: the three files nothing in CI checks

## Verification

<!-- Paste the unedited output of the commands you ran: unit tests, lint, and the acceptance tests for the packages you touched. -->
