## What this changes

<!-- One paragraph. Name the resource or the area, and the Dokploy behavior that drives the change. -->

## Checklist for every pull request

- [ ] Every commit is signed and shows **Verified** on GitHub ([how to sign](https://github.com/vanillauys/terraform-provider-dokploy/blob/master/CONTRIBUTING.md#sign-your-commits))
- [ ] `CHANGELOG.md`: a line under `## [Unreleased]`
- [ ] `make test`, `make lint` (0 issues), and `make docs` pass

## Checklist for a resource or data source change

- [ ] `internal/client/<name>.go` and its test: the endpoints and the wire shape
- [ ] `internal/client/dialect_test.go`: every new request struct
- [ ] `internal/resources/<name>/`: schema, CRUD, `ImportState`, and the model tests
- [ ] `*_acc_test.go`: a step that drops each optional attribute and asserts an empty plan; import with `ImportStateVerify`
- [ ] `internal/provider/provider.go`: the registration
- [ ] `examples/resources/dokploy_<name>/`: `resource.tf` and `import.sh`
- [ ] `make docs`: the generated page is committed
- [ ] `templates/index.md.tmpl` and `README.md`: the two files nothing in CI checks

## Verification

<!-- Paste the unedited output of the commands you ran: unit tests, lint, and the acceptance tests for the packages you touched. -->
