#!/usr/bin/env python3
"""Emit Terraform `import` blocks for every live resource this provider supports.

Read-only, like introspect.py: GET requests only, no POST helper exists here.
Output goes to stdout as HCL.

Also doubles, via `--patch-sensitive`, as the fix for a `-generate-config-out`
limitation dry-run.sh hits on every database engine: Terraform Core refuses
to write a value for any `Sensitive` schema attribute into generated config
(`null # sensitive` is emitted instead), and a `null` on a `Required`
attribute (`database_password`, on every engine) is then rejected before the
provider ever runs. Since this script already holds a read-only API key and
already knows how to map a Terraform resource address back to a live `.one`
record, it is the natural place to backfill those `null # sensitive` values
with what the live, read-only API already returns in plaintext (see
dogfood/README.md's "Known limitation" section for the full analysis of why
this is safe and not a new secrets-exposure surface).
"""

import json
import os
import re
import sys
import urllib.parse
import urllib.request

ENDPOINT = os.environ["DOKPLOY_ENDPOINT"].rstrip("/")
API_KEY = os.environ["DOKPLOY_API_KEY"]

# resource type -> (".one" endpoint, id field). Covers every resource type
# this generator ever emits an import block for, not just the database
# engines, so a Required+Sensitive attribute added to any future resource
# type is patched the same way without another hardcoded name.
ONE = {
    "dokploy_project": ("project.one", "projectId"),
    "dokploy_environment": ("environment.one", "environmentId"),
    "dokploy_application": ("application.one", "applicationId"),
    "dokploy_domain": ("domain.one", "domainId"),
    "dokploy_postgres": ("postgres.one", "postgresId"),
    "dokploy_mysql": ("mysql.one", "mysqlId"),
    "dokploy_mariadb": ("mariadb.one", "mariadbId"),
    "dokploy_mongo": ("mongo.one", "mongoId"),
    "dokploy_redis": ("redis.one", "redisId"),
}


def get(path, **params):
    url = f"{ENDPOINT}/api/{path}"
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, method="GET")
    req.add_header("x-api-key", API_KEY)
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def label(*parts):
    """A stable, valid Terraform identifier."""
    slug = "_".join(re.sub(r"[^a-zA-Z0-9]+", "_", p).strip("_") for p in parts)
    return slug.lower() or "unnamed"


def emit(resource_type, name, ident):
    print(f'import {{\n  to = {resource_type}.{name}\n  id = "{ident}"\n}}\n')


def snake_to_camel(name):
    """`database_password` -> `databasePassword`, matching this API's own
    camelCase JSON field naming (see internal/client/doc.go)."""
    head, *rest = name.split("_")
    return head + "".join(p.capitalize() for p in rest)


def hcl_string(value):
    """A safely quoted HCL string literal for an arbitrary live value.

    HCL string escaping matches JSON for backslash/quote/control characters,
    with one addition: `${` and `%{` introduce template syntax and must be
    doubled to be treated literally. json.dumps handles the former; the
    replace calls handle the latter.
    """
    s = json.dumps(value)
    return s.replace("${", "$${").replace("%{", "%%{")


def parse_imports(imports_path):
    """address -> id, from the `import { to = ...; id = "..." }` blocks this
    same script emits. Deliberately independent of generated.tf's own
    id-bearing comments (`# __generated__ by Terraform from "<id>"`), which
    Terraform omits precisely for resources that fail to generate fully --
    exactly the resources this function exists to help patch."""
    addr_to_id = {}
    pending_addr = None
    with open(imports_path) as f:
        for line in f:
            line = line.strip()
            if line.startswith("to ="):
                pending_addr = line.split("=", 1)[1].strip()
            elif line.startswith("id =") and pending_addr:
                addr_to_id[pending_addr] = line.split("=", 1)[1].strip().strip('"')
                pending_addr = None
    return addr_to_id


_RESOURCE_RE = re.compile(r'^resource\s+"([a-zA-Z0-9_]+)"\s+"([a-zA-Z0-9_]+)"\s*\{')
_SENSITIVE_NULL_RE = re.compile(r'^(\s*)([a-zA-Z0-9_]+)(\s*=\s*)null\s*#\s*sensitive\s*$')


def patch_sensitive(imports_path, generated_path):
    """Rewrite every `<attr> = null # sensitive` line in generated_path with
    the live value the corresponding `.one` endpoint already returns.

    Generic by construction: it does not name `database_password` or any
    other attribute anywhere in this function. It only recognizes the
    textual pattern Terraform's own config generation emits for ANY
    Sensitive attribute it could not populate, and backfills whichever
    attribute that turns out to be from the live record for the enclosing
    resource block (tracked by the most recently seen `resource "TYPE"
    "LABEL" {` line -- generated.tf's blocks never nest a resource inside
    another, so simple line-order tracking is sufficient here). A resource
    type this script doesn't recognize, or a live value that is itself null
    or missing, is left untouched rather than guessed at -- the next
    `terraform plan` surfaces that plainly if it still matters.
    """
    addr_to_id = parse_imports(imports_path)

    with open(generated_path) as f:
        lines = f.readlines()

    current_addr = None
    one_cache = {}
    patched = 0
    out = []
    for line in lines:
        m = _RESOURCE_RE.match(line)
        if m:
            current_addr = f"{m.group(1)}.{m.group(2)}"

        sm = _SENSITIVE_NULL_RE.match(line)
        if sm and current_addr:
            indent, attr, eq = sm.groups()
            resource_type = current_addr.split(".", 1)[0]
            rid = addr_to_id.get(current_addr)
            endpoint = ONE.get(resource_type)
            if rid and endpoint:
                if current_addr not in one_cache:
                    path, idfield = endpoint
                    try:
                        one_cache[current_addr] = get(path, **{idfield: rid})
                    except Exception as e:                      # noqa: BLE001
                        print(f"    !! {path} failed for {current_addr}: {e}",
                              file=sys.stderr)
                        one_cache[current_addr] = {}
                rec = one_cache[current_addr]
                key = snake_to_camel(attr)
                if isinstance(rec, dict) and rec.get(key) is not None:
                    line = (f"{indent}{attr}{eq}{hcl_string(rec[key])} "
                            f"# patched from live read (was unset/sensitive)\n")
                    patched += 1
        out.append(line)

    with open(generated_path, "w") as f:
        f.writelines(out)
    print(f"    patched {patched} sensitive attribute(s) with live values", file=sys.stderr)


def main():
    # Every label includes the resource's own ID. Names alone are not unique:
    # two environments in one project can share a name, and two domains on
    # one application can share a host (live-verified, see
    # .claude/skills/dokploy-api-quirks). A name-only label can therefore
    # collide across two distinct resources, which Terraform rejects as a
    # duplicate `to` address at the very first plan. The ID is the API's own
    # primary key, so appending it guarantees a unique label regardless of
    # what collides on the name/host alone. Applied to every resource type
    # here, not just the two documented collision cases, to keep the naming
    # scheme uniform.
    for project in get("project.all"):
        pid, pname = project["projectId"], project["name"]
        emit("dokploy_project", label(pname, pid), pid)

        for env in get("environment.byProjectId", projectId=pid):
            eid = env["environmentId"]
            emit("dokploy_environment", label(pname, env["name"], eid), eid)

            full = get("environment.one", environmentId=eid)
            for app in full.get("applications") or []:
                aid = app["applicationId"]
                emit("dokploy_application", label(pname, app["name"], aid), aid)
                for dom in get("domain.byApplicationId", applicationId=aid) or []:
                    emit("dokploy_domain", label(pname, dom["host"], dom["domainId"]), dom["domainId"])
            for pg in full.get("postgres") or []:
                emit("dokploy_postgres", label(pname, pg["name"], pg["postgresId"]), pg["postgresId"])
            for db in full.get("mysql") or []:
                emit("dokploy_mysql", label(pname, db["name"], db["mysqlId"]), db["mysqlId"])
            for db in full.get("mariadb") or []:
                emit("dokploy_mariadb", label(pname, db["name"], db["mariadbId"]), db["mariadbId"])
            for db in full.get("mongo") or []:
                emit("dokploy_mongo", label(pname, db["name"], db["mongoId"]), db["mongoId"])
            for db in full.get("redis") or []:
                emit("dokploy_redis", label(pname, db["name"], db["redisId"]), db["redisId"])


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--patch-sensitive":
        patch_sensitive(sys.argv[2], sys.argv[3])
    else:
        main()
