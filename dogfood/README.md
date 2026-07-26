# Dogfood harness

Read-only tooling that checks this provider against a **live** Dokploy server.
Neither script can modify the server: both issue HTTP GET only, and Dokploy
exposes every mutation as POST.

| Script | Purpose |
|---|---|
| `introspect.py` | Enumerate a server: projects, environments, services, domains, and their child resources. Secrets are reported as a length, never printed. |
| `generate_imports.py` | Emit Terraform `import` blocks for every live resource this provider supports. |
| `dry-run.sh` | Build the provider, import the live stack into a throwaway state, have Terraform generate the config, and require a second plan to be empty. |

## Running it

```bash
export DOKPLOY_ENDPOINT=https://your-dokploy-host
export DOKPLOY_API_KEY=...
DOKPLOY_DOGFOOD=1 ./dogfood/dry-run.sh
```

`dry-run.sh` never runs `terraform apply`, and deletes its scratch directory on
success. On failure it keeps `dogfood/scratch/` so the generated config can be
inspected.

This is **not** part of CI — it needs a real server, and CI only ever has a
disposable one.
