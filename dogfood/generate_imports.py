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
dogfood/README.md's "Database engines: the Required+Sensitive gap in
-generate-config-out, and its fix" section for the full analysis of why
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
    "dokploy_libsql": ("libsql.one", "libsqlId"),
    "dokploy_mount": ("mounts.one", "mountId"),
    "dokploy_port": ("port.one", "portId"),
    "dokploy_redirect": ("redirects.one", "redirectId"),
    "dokploy_security": ("security.one", "securityId"),
    "dokploy_destination": ("destination.one", "destinationId"),
    "dokploy_backup": ("backup.one", "backupId"),
    "dokploy_schedule": ("schedule.one", "scheduleId"),
    "dokploy_volume_backup": ("volumeBackups.one", "volumeBackupId"),
    "dokploy_compose": ("compose.one", "composeId"),
    "dokploy_network": ("network.one", "networkId"),
}

# (environment.one's collection key, resource type, id field). Drives the
# per-engine loop in main() below: every database engine only differs in
# these three strings, so a sixth engine is a one-line addition here rather
# than another copy-pasted loop body (wave-2 task 9 carry item C16).
DATABASE_ENGINES = [
    ("postgres", "dokploy_postgres", "postgresId"),
    ("mysql", "dokploy_mysql", "mysqlId"),
    ("mariadb", "dokploy_mariadb", "mariadbId"),
    ("mongo", "dokploy_mongo", "mongoId"),
    ("redis", "dokploy_redis", "redisId"),
    ("libsql", "dokploy_libsql", "libsqlId"),
]


def is_server_created_data_mount(service, mount):
    """A database engine's own data volume, which nothing asked for.

    Creating a dokploy_postgres (or mysql/mariadb/mongo/redis/libsql) makes
    Dokploy attach a volume mount for the container's data directory
    immediately -- verified live on the rig, v0.29.13, 2026-07-28: a freshly
    created postgres already owns volumeName "<appName>-data" at
    /var/lib/postgresql/18/docker. A fresh libsql was verified the same way
    (v0.29.13, 2026-08-12): it owns a volumeName "<appName>-data" mount too.

    It is an ordinary, removable mount, but it belongs to the server, not to
    anyone's configuration. Importing it would put a Terraform resource in
    charge of a volume the engine resource itself recreates, and destroying
    that resource would delete the database's data directory. So the
    generator skips it -- loudly, with a comment in imports.tf, never
    silently.

    The rule is checked, not guessed: type is volume AND volumeName is
    exactly the service's appName plus "-data".
    """
    return (
        mount.get("type") == "volume"
        and mount.get("volumeName") == f"{service.get('appName')}-data"
    )


def emit_mounts(prefix, service, mounts, *, skip_data_mount):
    """Emit import blocks for a service's mounts."""
    for mount in mounts or []:
        mid = mount["mountId"]
        if skip_data_mount and is_server_created_data_mount(service, mount):
            print(
                f"# skipped {mid}: Dokploy created this data volume "
                f"({mount.get('volumeName')}) with the service itself. It is not "
                f"user configuration, and a dokploy_mount managing it would delete "
                f"the database's data directory on destroy."
            )
            continue
        emit("dokploy_mount", label(prefix, mount.get("mountPath") or "mount", mid), mid)


# Recovered from each endpoint's own zod error (v0.29.13, 2026-07-28). They
# are deliberately different: a schedule runs a command somewhere, a volume
# backup archives a volume, and databases have volumes but do not run
# commands.
SCHEDULE_TYPES = {"application", "compose", "server", "dokploy-server"}
VOLUME_BACKUP_TYPES = {
    "application", "postgres", "mysql", "mariadb", "mongo", "redis", "compose", "libsql",
}


def compose_source_gap(detail):
    """Why a compose record cannot be imported, or None when it can.

    dokploy_compose models the github, git and raw sources and requires
    exactly one of them. A record whose source columns are still empty, or
    whose sourceType this provider does not model, has no valid Terraform
    configuration.
    """
    source = detail.get("sourceType")
    if source == "github":
        if not detail.get("repository"):
            return "has sourceType github but no repository yet; configure the source in Dokploy first"
    elif source == "git":
        if not detail.get("customGitUrl"):
            return "has sourceType git but no repository URL yet; configure the source in Dokploy first"
    elif source == "raw":
        if not detail.get("composeFile"):
            return "has sourceType raw but an empty compose file; add the file in Dokploy first"
    else:
        return f"has sourceType {source}, which this provider does not model"
    return None


def emit_backup_plane(prefix, service_type, service_id, detail):
    """Emit import blocks for a service's backups, schedules and volume backups.

    Three resources, three DIFFERENT discovery paths, because Dokploy is not
    consistent here (verified live, v0.29.13, 2026-07-28):

      backups        embedded in the parent's own record. There is no
                     backup.all and backup.create returns nothing, so this
                     array is the ONLY place a backup id is enumerated.
      schedules      NOT embedded -- the parent's `schedules` key is null even
                     when schedules exist. Needs schedule.list, which requires
                     id AND scheduleType.
      volumeBackups  NOT embedded either. Needs volumeBackups.list, which
                     requires id AND volumeBackupType.

    Reading only the parent record would silently miss two of the three.
    """
    for b in detail.get("backups") or []:
        bid = b["backupId"]
        emit("dokploy_backup", label(prefix, b.get("database") or "backup", bid), bid)

    # The two list endpoints validate their type against DIFFERENT enums, and
    # querying one with a type it does not accept is an HTTP 400, not an
    # empty list. Schedules attach only to applications, compose services and
    # servers -- never to a database -- while volume backups attach to any
    # service with a volume. Guard on each enum rather than discovering the
    # difference as a crash.
    if service_type in SCHEDULE_TYPES:
        for sc in get("schedule.list", id=service_id, scheduleType=service_type) or []:
            sid = sc["scheduleId"]
            emit("dokploy_schedule", label(prefix, sc.get("name") or "schedule", sid), sid)

    if service_type in VOLUME_BACKUP_TYPES:
        for vb in get("volumeBackups.list", id=service_id, volumeBackupType=service_type) or []:
            vid = vb["volumeBackupId"]
            emit("dokploy_volume_backup", label(prefix, vb.get("name") or "volume", vid), vid)


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
    `terraform plan` surfaces that plainly if it still matters. An EMPTY live
    value is likewise left alone: see the comment at the check below.
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
                # An EMPTY live value must be left as null, not written as "".
                #
                # The provider maps both JSON null and "" to a null Terraform
                # value on read (tfutil.StringOrNull), because Dokploy uses
                # them interchangeably for "unset". Writing `attr = ""` into
                # the config therefore produces a permanent `null -> ""` diff
                # that no apply can settle. Only Optional sensitive
                # attributes can reach here with an empty value -- a Required
                # one is never legitimately blank -- and for those, null IS
                # the correct encoding of unset, which is already what
                # Terraform wrote.
                #
                # Found by wave 3's first production round-trip:
                # build_secrets is empty on both live applications, and
                # patching it to "" left two resources permanently diffing.
                if isinstance(rec, dict) and rec.get(key) not in (None, ""):
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

                # The child collections are only reachable through the
                # parent: there is no port.all / redirects.all / security.all,
                # and redirects.create/security.create do not even return the
                # records they make.
                detail = get("application.one", applicationId=aid)
                for port in detail.get("ports") or []:
                    emit("dokploy_port", label(pname, app["name"], port["portId"]), port["portId"])
                for red in detail.get("redirects") or []:
                    emit("dokploy_redirect", label(pname, app["name"], red["redirectId"]), red["redirectId"])
                for sec in detail.get("security") or []:
                    emit("dokploy_security", label(pname, app["name"], sec["securityId"]), sec["securityId"])
                # An application has no auto-created data mount, so nothing
                # is skipped here.
                emit_mounts(f"{pname}-{app['name']}", detail, detail.get("mounts"),
                            skip_data_mount=False)
                emit_backup_plane(f"{pname}-{app['name']}", "application", aid, detail)

            # compose.one embeds its domains, unlike application.one, so no
            # domain.byComposeId call exists or is needed (checked on the
            # rig, v0.30.5, 2026-09-05). A compose service has no
            # auto-created data mount.
            for comp in full.get("compose") or []:
                cid = comp["composeId"]
                detail = get("compose.one", composeId=cid)
                reason = compose_source_gap(detail)
                if reason:
                    # dokploy_compose requires exactly one source block, so a
                    # record without a usable source has no valid configuration:
                    # `-generate-config-out` writes a block of null Required
                    # attributes and the second plan errors (seen live,
                    # v0.30.5, 2026-09-05, on a compose created through the API
                    # and never configured).
                    print(f"# skipped {cid}: dokploy_compose {label(pname, comp['name'], cid)} {reason}")
                    continue
                emit("dokploy_compose", label(pname, comp["name"], cid), cid)
                for dom in detail.get("domains") or []:
                    emit("dokploy_domain", label(pname, dom["host"], dom["domainId"]), dom["domainId"])
                emit_mounts(f"{pname}-{comp['name']}", detail, detail.get("mounts"),
                            skip_data_mount=False)
                emit_backup_plane(f"{pname}-{comp['name']}", "compose", cid, detail)

            for collection_key, resource_type, id_key in DATABASE_ENGINES:
                for db in full.get(collection_key) or []:
                    emit(resource_type, label(pname, db["name"], db[id_key]), db[id_key])
                    endpoint, param = ONE[resource_type]
                    detail = get(endpoint, **{param: db[id_key]})
                    emit_mounts(f"{pname}-{db['name']}", detail, detail.get("mounts"),
                                skip_data_mount=True)
                    emit_backup_plane(f"{pname}-{db['name']}", collection_key,
                                      db[id_key], detail)

    for dest in get("destination.all") or []:
        did = dest["destinationId"]
        emit("dokploy_destination", label(dest["name"], did), did)

    # Networks are global, like destinations.
    for net in get("network.all") or []:
        nid = net["networkId"]
        emit("dokploy_network", label(net["name"], nid), nid)

    # Vault providers are global too, but the server redacts every config
    # secret on read, so `terraform import` leaves the config block null and
    # the first apply after the import writes a full-body update. An import
    # block here would therefore break dry-run.sh's empty second plan. The
    # record is listed as a comment: import it by hand and supply the config
    # block for its type, as the dokploy_vault_provider import notes say.
    for vp in get("vaultProvider.all") or []:
        vid = vp["vaultProviderId"]
        print(
            f"# not imported: dokploy_vault_provider {label(vp['name'], vid)} "
            f'(id "{vid}", type {vp.get("providerType")}). The server redacts the config '
            f"block, so import it by hand and supply the block for its type."
        )


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--patch-sensitive":
        # Not reachable via dry-run.sh's own (fixed) invocation; guarded so a
        # manual or future call gets a clear usage error instead of an
        # unhandled IndexError (wave-2 task 9 carry item C18).
        if len(sys.argv) != 4:
            sys.exit("usage: generate_imports.py --patch-sensitive <imports.tf> <generated.tf>")
        patch_sensitive(sys.argv[2], sys.argv[3])
    else:
        main()
