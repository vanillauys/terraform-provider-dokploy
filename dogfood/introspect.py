#!/usr/bin/env python3
"""
Read-only introspection of a live Dokploy server.

Enumerates what the stack actually uses, so wave 1 of terraform-provider-dokploy
can be scoped against reality instead of a guess.

SAFETY: this script can only issue HTTP GET. There is no POST helper in it at
all, and Dokploy exposes every mutation (.create/.update/.remove/.delete/
.deploy) as POST only -- so it is structurally incapable of changing anything.

Secrets (env blocks, passwords, tokens) are never printed; only their presence
and size.

Usage:
    export DOKPLOY_ENDPOINT=https://<your-dokploy-host>
    export DOKPLOY_API_KEY=<key>          # stays in your shell
    python3 introspect-live.py
"""

import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

ENDPOINT = os.environ.get("DOKPLOY_ENDPOINT", "").rstrip("/")
API_KEY = os.environ.get("DOKPLOY_API_KEY", "")

if not ENDPOINT or not API_KEY:
    sys.exit("set DOKPLOY_ENDPOINT and DOKPLOY_API_KEY first")

# The service collections Dokploy embeds in an environment record.
SERVICE_KINDS = ["applications", "postgres", "mysql", "mariadb", "mongo",
                 "redis", "libsql", "compose"]

# <kind> -> (one-endpoint, id-field)
ONE = {
    "applications": ("application.one", "applicationId"),
    "postgres": ("postgres.one", "postgresId"),
    "mysql": ("mysql.one", "mysqlId"),
    "mariadb": ("mariadb.one", "mariadbId"),
    "mongo": ("mongo.one", "mongoId"),
    "redis": ("redis.one", "redisId"),
    "libsql": ("libsql.one", "libsqlId"),
    "compose": ("compose.one", "composeId"),
}


def get(path, **params):
    """Issue a GET. This is the only HTTP verb this script can perform."""
    url = f"{ENDPOINT}/api/{path}"
    if params:
        url += "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, method="GET")
    req.add_header("x-api-key", API_KEY)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return json.loads(r.read())
    except urllib.error.HTTPError as e:
        return {"__error__": f"HTTP {e.code}"}
    except Exception as e:                                    # noqa: BLE001
        return {"__error__": type(e).__name__}


def secret(v):
    """Describe a secret without revealing it."""
    if v is None:
        return "none"
    s = str(v)
    if not s:
        return "empty"
    return f"<{len(s)} chars, {len(s.splitlines())} lines>"


def show_domains(domains, indent):
    pad = " " * indent
    if not domains:
        print(f"{pad}domains:   (none)")
        return
    for d in domains:
        bits = [
            f"host={d.get('host')}",
            f"port={d.get('port')}",
            f"https={d.get('https')}",
            f"cert={d.get('certificateType')}",
        ]
        if d.get("path") not in (None, "/"):
            bits.append(f"path={d.get('path')}")
        if d.get("internalPath") not in (None, "/"):
            bits.append(f"internalPath={d.get('internalPath')}")
        if d.get("stripPath"):
            bits.append("stripPath=true")
        if d.get("customEntrypoint"):
            bits.append(f"entrypoint={d.get('customEntrypoint')}")
        if d.get("customCertResolver"):
            bits.append(f"resolver={d.get('customCertResolver')}")
        if d.get("middlewares"):
            bits.append(f"middlewares={d.get('middlewares')}")
        if d.get("forwardAuthEnabled"):
            bits.append("forwardAuth=true")
        if d.get("serviceName"):
            bits.append(f"serviceName={d.get('serviceName')}")
        print(f"{pad}domain:    " + "  ".join(bits))


def show_children(rec, indent):
    """Print the child collections that would each need their own resource."""
    pad = " " * indent
    for key, label in [("ports", "ports"), ("redirects", "redirects"),
                       ("security", "security"), ("mounts", "mounts")]:
        items = rec.get(key) or []
        if not items:
            continue
        print(f"{pad}{label + ':':10s} {len(items)}")
        for it in items:
            if key == "ports":
                print(f"{pad}  published={it.get('publishedPort')} "
                      f"target={it.get('targetPort')} proto={it.get('protocol')}")
            elif key == "redirects":
                print(f"{pad}  {it.get('regex')} -> {it.get('replacement')} "
                      f"permanent={it.get('permanent')}")
            elif key == "security":
                print(f"{pad}  username={it.get('username')} "
                      f"password={secret(it.get('password'))}")
            elif key == "mounts":
                # Databases auto-create their own data volume; that one is
                # server-managed and is not something Terraform would declare.
                auto = " (auto-created data volume)" if str(
                    it.get("volumeName") or "").endswith("-data") else ""
                print(f"{pad}  type={it.get('type')} "
                      f"mountPath={it.get('mountPath')} "
                      f"volumeName={it.get('volumeName')} "
                      f"hostPath={it.get('hostPath')}{auto}")


def main():
    print(f"Dokploy introspection: {ENDPOINT}")
    print("=" * 72)

    projects = get("project.all")
    if isinstance(projects, dict):
        sys.exit(f"project.all failed: {projects.get('__error__')}")

    totals = {"projects": 0, "environments": 0, "domains": 0,
              "ports": 0, "redirects": 0, "security": 0, "mounts": 0}
    kind_counts = {}

    for p in projects:
        totals["projects"] += 1
        print(f"\nPROJECT  {p.get('name')}")
        print(f"  projectId    {p.get('projectId')}")
        print(f"  description  {p.get('description')!r}")
        print(f"  env          {secret(p.get('env'))}")

        envs = get("environment.byProjectId", projectId=p.get("projectId"))
        if isinstance(envs, dict):
            print(f"  !! environment.byProjectId failed: {envs.get('__error__')}")
            continue

        for e in envs:
            totals["environments"] += 1
            flag = " [DEFAULT]" if e.get("isDefault") else ""
            print(f"\n  ENVIRONMENT  {e.get('name')}{flag}")
            print(f"    environmentId  {e.get('environmentId')}")
            print(f"    description    {e.get('description')!r}")
            print(f"    env            {secret(e.get('env'))}")

            for kind in SERVICE_KINDS:
                for svc in (e.get(kind) or []):
                    kind_counts[kind] = kind_counts.get(kind, 0) + 1
                    endpoint, idfield = ONE[kind]
                    rec = get(endpoint, **{idfield: svc.get(idfield)})
                    if isinstance(rec, dict) and rec.get("__error__"):
                        print(f"    {kind}: {svc.get('name')} "
                              f"!! {endpoint} failed: {rec['__error__']}")
                        continue

                    label = "APPLICATION" if kind == "applications" else kind.upper()
                    print(f"\n    {label}  {rec.get('name')}")
                    print(f"      appName      {rec.get('appName')}")
                    if kind == "applications":
                        print(f"      sourceType   {rec.get('sourceType')}")
                        print(f"      buildType    {rec.get('buildType')}")
                        for f in ("repository", "owner", "branch", "buildPath",
                                  "dockerImage", "customGitUrl", "dockerfile",
                                  "replicas", "autoDeploy", "triggerType"):
                            if rec.get(f) not in (None, ""):
                                print(f"      {f:12s} {rec.get(f)}")
                        for f in ("memoryLimit", "memoryReservation",
                                  "cpuLimit", "cpuReservation", "command",
                                  "healthCheckSwarm", "labelsSwarm",
                                  "publishDirectory", "isStaticSpa"):
                            if rec.get(f) not in (None, ""):
                                print(f"      {f:12s} {rec.get(f)!r}")
                    elif kind == "compose":
                        print(f"      composeType  {rec.get('composeType')}")
                        print(f"      sourceType   {rec.get('sourceType')}")
                        print(f"      composeFile  {secret(rec.get('composeFile'))}")
                    else:
                        print(f"      dockerImage  {rec.get('dockerImage')}")
                        print(f"      externalPort {rec.get('externalPort')}")
                        for f in ("databaseName", "databaseUser"):
                            if f in rec:
                                print(f"      {f:12s} {rec.get(f)}")
                    print(f"      env          {secret(rec.get('env'))}")
                    print(f"      status       {rec.get('applicationStatus')}")
                    print(f"      serverId     {rec.get('serverId')}")

                    doms = rec.get("domains") or []
                    totals["domains"] += len(doms)
                    show_domains(doms, 6)
                    for k in ("ports", "redirects", "security", "mounts"):
                        totals[k] += len(rec.get(k) or [])
                    show_children(rec, 6)

    certs = get("certificates.all")
    print("\n" + "=" * 72)
    if isinstance(certs, list):
        print(f"CERTIFICATES  {len(certs)}")
        for c in certs:
            print(f"  name={c.get('name')} "
                  f"autoRenew={c.get('autoRenew')} "
                  f"serverId={c.get('serverId')}")

    print("\nTOTALS")
    for k, v in totals.items():
        print(f"  {k:14s} {v}")
    for k, v in sorted(kind_counts.items()):
        print(f"  {k:14s} {v}")

    print("\nWAVE-1 IMPLICATION")
    have = {"applications", "postgres"}          # shipped in v0.1.0
    need = []
    if totals["environments"] > totals["projects"]:
        need.append("dokploy_environment (non-default environments in use)")
    else:
        need.append("dokploy_environment (only default envs -- import-only path matters most)")
    if totals["domains"]:
        need.append(f"dokploy_domain ({totals['domains']} in use)")
    for k, res in [("ports", "dokploy_port"), ("redirects", "dokploy_redirect"),
                   ("security", "dokploy_security"), ("mounts", "dokploy_mount")]:
        if totals[k]:
            need.append(f"{res} ({totals[k]} in use)")
    for k in sorted(kind_counts):
        if k not in have and k != "applications":
            need.append(f"dokploy_{k} ({kind_counts[k]} in use)")
    for n in need:
        print(f"  - {n}")


if __name__ == "__main__":
    main()
