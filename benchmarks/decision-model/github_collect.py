import argparse
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path


REPOSITORIES = (
    "pallets/flask", "pallets/click", "pallets/werkzeug",
    "encode/httpx", "pytest-dev/pytest", "Textualize/rich", "psf/requests",
)


def collect(repository):
    owner, name = repository.split("/")
    query = '''query($owner:String!,$name:String!) {
      repository(owner:$owner,name:$name) {
        isPrivate licenseInfo { spdxId }
        pullRequests(first:60,states:MERGED,orderBy:{field:UPDATED_AT,direction:DESC}) {
          nodes { number url title mergedAt baseRefOid mergeCommit { oid }
            closingIssuesReferences(first:3) { nodes { number url title body updatedAt } }
            files(first:100) { nodes { path } totalCount }
          }
        }
      }
    }'''
    result = subprocess.run(
        ["gh", "api", "graphql", "-f", "query=" + query,
         "-f", "owner=" + owner, "-f", "name=" + name],
        capture_output=True, text=True, encoding="utf-8", check=True)
    payload = json.loads(result.stdout)
    if payload.get("errors"):
        raise RuntimeError("GitHub GraphQL returned errors")
    source = payload["data"]["repository"]
    if source["isPrivate"]:
        raise ValueError("Collector only accepts public repositories")
    return {"repository": repository, "retrieved_at": datetime.now(timezone.utc).isoformat(),
            "repository_license_spdx": (source["licenseInfo"] or {}).get("spdxId"),
            "pull_requests": source["pullRequests"]["nodes"]}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    for repository in REPOSITORIES:
        path = args.output / (repository.replace("/", "--") + ".json")
        if path.exists():
            raise ValueError("Snapshot exists; choose a new output directory")
        snapshot = collect(repository)
        path.write_text(json.dumps(snapshot, ensure_ascii=False, indent=2), encoding="utf-8")
        linked = sum(bool(p["closingIssuesReferences"]["nodes"]) for p in snapshot["pull_requests"])
        print(f"{repository}: {len(snapshot['pull_requests'])} merged PRs, {linked} with linked issues", flush=True)


if __name__ == "__main__":
    main()
