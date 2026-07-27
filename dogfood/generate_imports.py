#!/usr/bin/env python3
"""Emit Terraform `import` blocks for every live resource this provider supports.

Read-only, like introspect.py: GET requests only, no POST helper exists here.
Output goes to stdout as HCL.
"""

import json
import os
import re
import urllib.parse
import urllib.request

ENDPOINT = os.environ["DOKPLOY_ENDPOINT"].rstrip("/")
API_KEY = os.environ["DOKPLOY_API_KEY"]


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
    main()
