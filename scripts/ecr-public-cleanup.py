#!/usr/bin/env python3
"""
Delete images from Tensorleap's public ECR registry on request.

ECR Public has no lifecycle policies (aws/containers-roadmap#1268), so every
branch build that CI pushes to public.ecr.aws/tensorleap stays until something
deletes it. This script is that something: it lists a repository, classifies
every image by its tags, and expires the classes you ask for once they are at
least N days old. Only engine, engine-generic, node-server and web-ui can be
cleaned; `--repos all` (the default) means those four. N can never be below 90:
anything younger is assumed to still matter to somebody.

Classes (the strictest one wins per image, because deleting a digest removes
ALL of its tags):

  version   any tag matches ^v?\\d+\\.\\d+   (1.6.72-41be9147, 1.6.72-stable,
            k3s:v1.26.4-...)                 NEVER deletable, no flag overrides
  master    any tag starts with master-      deletable only when asked for
  feature   any other tagged image           (domain_gap-abc12345, 72-patch-all-latest)
  untagged  no tags at all                   orphans left when CI re-runs a commit

Feature branches cut from a version branch (1.6.72-rc-test-<sha>) also match
the version rule and are protected. That is an accepted false positive.

Nothing is deleted unless BOTH --no-dry-run and --confirm "<phrase>" are given;
DRY_RUN=1 in the environment forces a dry run regardless. Every run writes the
candidate list, the protected/kept list and a per-repo summary to --out.

Auth is whatever the AWS CLI resolves: AWS_PROFILE=utils locally, static keys or
an assumed role in CI. The ECR Public API only exists in us-east-1.

Usage:
  # plan only (default): feature-branch and untagged images at least 90 days old, all four repos
  AWS_PROFILE=utils scripts/ecr-public-cleanup.py --classes feature,untagged --older-than-days 90

  # one repo, master images too
  AWS_PROFILE=utils scripts/ecr-public-cleanup.py \\
      --repos web-ui --classes feature,untagged,master --older-than-days 180

  # really delete, keeping the tags listed in keep.txt (one repo:glob per line)
  AWS_PROFILE=utils scripts/ecr-public-cleanup.py \\
      --repos node-server,web-ui --classes feature,untagged --older-than-days 90 \\
      --keep-file keep.txt --no-dry-run --confirm "DELETE FROM PUBLIC ECR"

  scripts/ecr-public-cleanup.py --self-test     # classification asserts, no AWS calls
"""

from __future__ import annotations

import argparse
import csv
import fnmatch
import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

VERSION_RE = re.compile(r"^v?\d+\.\d+")
MASTER_PREFIX = "master-"
INDEX_MEDIA_TYPES = {
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
}
CONFIRM_PHRASE = "DELETE FROM PUBLIC ECR"
BATCH_SIZE = 100  # batch-delete-image hard maximum
REGION = "us-east-1"  # the only region the ECR Public API exists in
DELETABLE_CLASSES = ("feature", "master", "untagged")
# The only repositories this tool may touch; `--repos all` means these four.
CLEANABLE_REPOS = ("engine", "engine-generic", "node-server", "web-ui")
# Hard floor: nothing younger than this can be deleted, whatever the caller asks.
MIN_OLDER_THAN_DAYS = 90
DEFAULT_OLDER_THAN_DAYS = MIN_OLDER_THAN_DAYS
DEFAULT_OUT = "ecr-public-cleanup-out"
RETRYABLE_ERRORS = ("Throttling", "TooManyRequests", "RequestLimitExceeded")
CSV_COLUMNS = ["repo", "digest", "class", "age_days", "pushed_at", "size_bytes",
               "media_type", "is_index", "tags", "action"]
STAT_KEYS = ["scanned", "version", "master", "feature", "untagged", "candidates", "naive_bytes",
             "skipped_keep", "deleted", "already_gone", "skipped_referenced", "failed"]
CANDIDATE_ACTIONS = {"candidate", "deleted", "already-gone", "skipped-referenced", "failed"}
AUDIT_ACTIONS = {"skipped-protected", "skipped-keep"}


def log(msg: str) -> None:
    print(msg, flush=True)


def die(msg: str) -> None:
    print(f"❌ {msg}", file=sys.stderr, flush=True)
    sys.exit(1)


# -------------------------------------------------------------------- AWS ----

def aws(*args: str) -> dict:
    """Run one `aws ecr-public` call and return its JSON. Retries throttling."""
    env = {**os.environ, "AWS_RETRY_MODE": "adaptive", "AWS_MAX_ATTEMPTS": "10", "AWS_PAGER": ""}
    cmd = ["aws", "ecr-public", *args, "--region", REGION, "--output", "json"]
    for attempt in range(1, 6):
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True, env=env)
        except FileNotFoundError:
            die("aws CLI not found on PATH")
        if proc.returncode == 0:
            return json.loads(proc.stdout) if proc.stdout.strip() else {}
        if attempt < 5 and any(e in proc.stderr for e in RETRYABLE_ERRORS):
            time.sleep(2 ** attempt)
            continue
        die(f"aws ecr-public {args[0]} failed: {proc.stderr.strip()}")
    return {}  # unreachable, die() exits


def describe_images(repo: str) -> list[dict]:
    # The CLI follows nextToken itself; 1000 is the API maximum per page.
    return aws("describe-images", "--repository-name", repo, "--page-size", "1000").get("imageDetails", [])


# ---------------------------------------------------------- classification ----

def classify(tags) -> str:
    tags = tags or []
    if not tags:
        return "untagged"
    if any(VERSION_RE.match(t) for t in tags):
        return "version"
    if any(t.startswith(MASTER_PREFIX) for t in tags):
        return "master"
    return "feature"


def load_keep_globs(path: str) -> dict[str, list[str]]:
    """keep-file: one `repo:glob` per line, `#` comments; glob is fnmatch on a tag."""
    keep: dict[str, list[str]] = {}
    for lineno, raw in enumerate(Path(path).read_text().splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if ":" not in line:
            die(f"{path}:{lineno}: expected repo:glob, got '{line}'")
        repo, glob = line.split(":", 1)
        keep.setdefault(repo.strip(), []).append(glob.strip())
    return keep


def is_kept(repo: str, tags, keep: dict[str, list[str]]) -> bool:
    globs = keep.get(repo, [])
    return any(fnmatch.fnmatchcase(t, g) for t in (tags or []) for g in globs)


def parse_time(value: str) -> datetime:
    dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
    return dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)


def new_stats() -> dict:
    return {k: 0 for k in STAT_KEYS}


def plan_repo(repo: str, images: list[dict], opts, now: datetime) -> tuple[list[dict], dict]:
    """Classify every image and mark the ones this run may delete."""
    stats = new_stats()
    records = []
    for img in images:
        tags = sorted(img.get("imageTags") or [])
        cls = classify(tags)
        pushed = parse_time(img["imagePushedAt"])
        rec = {
            "repo": repo,
            "digest": img["imageDigest"],
            "class": cls,
            "age_days": int((now - pushed).total_seconds() // 86400),
            "pushed_at": pushed.isoformat(),
            "size_bytes": int(img.get("imageSizeInBytes") or 0),
            "media_type": img.get("imageManifestMediaType", ""),
            "is_index": img.get("imageManifestMediaType") in INDEX_MEDIA_TYPES,
            "tags": tags,
            "action": "",
        }
        stats["scanned"] += 1
        stats[cls] += 1
        if cls == "version":
            rec["action"] = "skipped-protected"
        elif cls not in opts.classes:
            rec["action"] = "skipped-class"
        elif rec["age_days"] < opts.older_than_days:
            rec["action"] = "skipped-age"
        elif is_kept(repo, tags, opts.keep):
            rec["action"] = "skipped-keep"
            stats["skipped_keep"] += 1
        else:
            rec["action"] = "candidate"
            stats["candidates"] += 1
            stats["naive_bytes"] += rec["size_bytes"]
        records.append(rec)
    return records, stats


# ----------------------------------------------------------------- delete ----

def order_for_delete(cands: list[dict]) -> list[dict]:
    # Indexes first: ECR Public refuses to delete a child manifest while an index
    # still references it (ImageReferencedByManifestList), so every index in a
    # repo has to go in an earlier batch than any single-arch child.
    return sorted(cands, key=lambda r: (not r["is_index"], r["pushed_at"]))


def delete_candidates(repo: str, cands: list[dict], stats: dict) -> None:
    ordered = order_for_delete(cands)
    batches = [ordered[i:i + BATCH_SIZE] for i in range(0, len(ordered), BATCH_SIZE)]
    for n, batch in enumerate(batches, 1):
        payload = {"repositoryName": repo, "imageIds": [{"imageDigest": r["digest"]} for r in batch]}
        resp = aws("batch-delete-image", "--cli-input-json", json.dumps(payload))
        failures = {f.get("imageId", {}).get("imageDigest"): f for f in resp.get("failures", [])}
        deleted = 0
        for rec in batch:
            failure = failures.get(rec["digest"])
            code = failure.get("failureCode") if failure else None
            if failure is None:
                rec["action"] = "deleted"
                stats["deleted"] += 1
                deleted += 1
            elif code == "ImageNotFound":  # deleted by someone else meanwhile
                rec["action"] = "already-gone"
                stats["already_gone"] += 1
            elif code == "ImageReferencedByManifestList":  # its index survived, leave it
                rec["action"] = "skipped-referenced"
                stats["skipped_referenced"] += 1
            else:
                rec["action"] = "failed"
                rec["failure"] = f"{code}: {failure.get('failureReason')}"
                stats["failed"] += 1
        log(f"🗑️  {repo}: batch {n}/{len(batches)} deleted {deleted}, failures {len(failures)}")


# ---------------------------------------------------------------- reports ----

def _gb(n: int) -> str:
    return f"{n / 1e9:,.0f}"


def render_summary_md(summary: dict) -> str:
    mode = "DRY RUN, nothing deleted" if summary["mode"] == "dry-run" else "REAL DELETION"
    lines = [
        f"## ECR Public cleanup: {mode}",
        "",
        f"- classes: `{','.join(summary['classes'])}` | older than: `{summary['older_than_days']}` days"
        f" | keep globs: {summary['keep_globs']}",
        "- `version` images (tags like `1.6.72-...`) are hard-protected and never deleted."
        " `naive_GB` sums `imageSizeInBytes` per manifest; shared layers are counted more than once,"
        " so the real storage drop is smaller.",
    ]
    lines += [
        "",
        "| repo | scanned | version | master | feature | untagged | candidates | naive_GB"
        " | skipped_keep | deleted | already_gone | skipped_referenced | failed |",
        "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
    ]

    def row(name: str, s: dict) -> str:
        return (f"| {name} | {s['scanned']} | {s['version']} | {s['master']} | {s['feature']} | {s['untagged']}"
                f" | {s['candidates']} | {_gb(s['naive_bytes'])} | {s['skipped_keep']} | {s['deleted']}"
                f" | {s['already_gone']} | {s['skipped_referenced']} | {s['failed']} |")

    for repo, s in summary["repos"].items():
        lines.append(row(repo, s))
    lines.append(row("**total**", summary["totals"]))
    return "\n".join(lines) + "\n"


def write_outputs(out_dir: Path, records: list[dict], stats_by_repo: dict[str, dict], opts, dry_run: bool) -> str:
    out_dir.mkdir(parents=True, exist_ok=True)
    cands = [r for r in records if r["action"] in CANDIDATE_ACTIONS]
    audit = [r for r in records if r["action"] in AUDIT_ACTIONS]
    for name, rows in (("candidates.csv", cands), ("skipped.csv", audit)):
        with (out_dir / name).open("w", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(CSV_COLUMNS)
            for r in rows:
                writer.writerow([r["repo"], r["digest"], r["class"], r["age_days"], r["pushed_at"], r["size_bytes"],
                                 r["media_type"], int(r["is_index"]), " ".join(r["tags"]), r["action"]])
    (out_dir / "candidates.json").write_text(json.dumps(cands, indent=1))
    totals = new_stats()
    for s in stats_by_repo.values():
        for k in STAT_KEYS:
            totals[k] += s[k]
    summary = {
        "mode": "dry-run" if dry_run else "delete",
        "classes": sorted(opts.classes),
        "older_than_days": opts.older_than_days,
        "keep_globs": sum(len(v) for v in opts.keep.values()),
        "repos": stats_by_repo,
        "totals": totals,
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=1))
    md = render_summary_md(summary)
    (out_dir / "summary.md").write_text(md)
    return md


# ------------------------------------------------------------------- main ----

def self_test() -> None:
    assert classify(["1.6.72-41be9147"]) == "version"
    assert classify(["1.6.72-stable", "1.6.72-41be9147-stable"]) == "version"
    assert classify(["v1.26.4-k3s1-cuda-11.8.0-ubuntu-22.04-v1"]) == "version"
    assert classify(["1.6.72-rc-test-abc12345"]) == "version"  # accepted false positive
    assert classify(["master-41be9147-py38-arm"]) == "master"
    assert classify(["master-abc12345", "domain_gap-abc12345"]) == "master"  # strictest wins
    assert classify(["domain_gap-abc12345"]) == "feature"
    assert classify(["72-patch-all-e55771ee", "72-patch-all-latest"]) == "feature"
    assert classify(["1786608682-386979-277db63e"]) == "feature"
    assert classify([]) == "untagged" and classify(None) == "untagged"

    keep_globs = {"engine": ["master-41be9147*"], "engine-generic": ["master-41be9147*"]}
    assert is_kept("engine", ["master-41be9147-amd64"], keep_globs)
    assert is_kept("engine-generic", ["master-41be9147-py38-arm"], keep_globs)
    assert not is_kept("engine", ["master-99999999"], keep_globs)
    assert not is_kept("node-server", ["master-41be9147"], keep_globs)

    idx = {"is_index": True, "pushed_at": "2026-02-01T00:00:00+00:00"}
    child = {"is_index": False, "pushed_at": "2026-01-01T00:00:00+00:00"}
    assert order_for_delete([child, idx]) == [idx, child]

    class Opts:
        classes = {"feature", "untagged"}
        older_than_days = 60
        keep = keep_globs

    now = datetime(2026, 9, 14, tzinfo=timezone.utc)
    images = [
        {"imageDigest": "sha256:a", "imageTags": ["1.6.72-abc12345"], "imagePushedAt": "2025-01-01T00:00:00+00:00",
         "imageSizeInBytes": 10, "imageManifestMediaType": "application/vnd.oci.image.index.v1+json"},
        {"imageDigest": "sha256:b", "imageTags": ["master-41be9147"], "imagePushedAt": "2025-01-01T00:00:00+00:00",
         "imageSizeInBytes": 10, "imageManifestMediaType": "application/vnd.oci.image.index.v1+json"},
        {"imageDigest": "sha256:c", "imageTags": ["feat-x-abc12345"], "imagePushedAt": "2025-01-01T00:00:00+00:00",
         "imageSizeInBytes": 10, "imageManifestMediaType": "application/vnd.oci.image.index.v1+json"},
        {"imageDigest": "sha256:d", "imageTags": ["feat-y-abc12345"], "imagePushedAt": "2026-09-01T00:00:00+00:00",
         "imageSizeInBytes": 10, "imageManifestMediaType": "application/vnd.oci.image.index.v1+json"},
        {"imageDigest": "sha256:e", "imagePushedAt": "2025-01-01T00:00:00+00:00", "imageSizeInBytes": 10,
         "imageManifestMediaType": "application/vnd.oci.image.manifest.v1+json"},
    ]
    records, stats = plan_repo("engine", images, Opts, now)
    actions = {r["digest"]: r["action"] for r in records}
    assert actions == {"sha256:a": "skipped-protected", "sha256:b": "skipped-class", "sha256:c": "candidate",
                       "sha256:d": "skipped-age", "sha256:e": "candidate"}, actions
    assert stats["candidates"] == 2 and stats["version"] == 1 and stats["naive_bytes"] == 20

    Opts.classes = {"master"}
    records, stats = plan_repo("engine", images, Opts, now)
    assert {r["digest"]: r["action"] for r in records}["sha256:b"] == "skipped-keep" and stats["skipped_keep"] == 1

    assert check_older_than_days(MIN_OLDER_THAN_DAYS) == MIN_OLDER_THAN_DAYS
    for too_low in (0, 1, MIN_OLDER_THAN_DAYS - 1):
        try:
            check_older_than_days(too_low)
        except ValueError:
            pass
        else:
            raise AssertionError(f"{too_low} days must be rejected")

    assert resolve_repos("all") == list(CLEANABLE_REPOS)
    assert resolve_repos(" web-ui, engine ") == ["web-ui", "engine"]
    log("✅ self-test passed")


def check_older_than_days(value: int) -> int:
    if value < MIN_OLDER_THAN_DAYS:
        raise ValueError(f"--older-than-days {value} is below the hard minimum of {MIN_OLDER_THAN_DAYS};"
                         " younger images are never deleted")
    return value


def resolve_repos(value: str) -> list[str]:
    if value.strip() == "all":
        return list(CLEANABLE_REPOS)
    repos = [r.strip() for r in value.split(",") if r.strip()]
    bad = [r for r in repos if r not in CLEANABLE_REPOS]
    if bad:
        die(f"cannot clean {bad}; this tool only touches {', '.join(CLEANABLE_REPOS)} (or 'all')")
    if not repos:
        die("--repos is empty")
    return repos


def parse_args(argv: list[str]) -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Delete images from Tensorleap's public ECR registry (dry run by default).",
        epilog=__doc__.split("Usage:", 1)[1], formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--repos", default="all",
                   help=f"comma-separated subset of {','.join(CLEANABLE_REPOS)}, or 'all' for those four (default)")
    p.add_argument("--classes", default="feature",
                   help=f"comma-separated classes to delete from {', '.join(DELETABLE_CLASSES)} (default: feature)."
                        " 'version' is never accepted.")
    p.add_argument("--older-than-days", type=int, default=DEFAULT_OLDER_THAN_DAYS,
                   help=f"only images pushed at least N days ago (default and hard minimum {MIN_OLDER_THAN_DAYS})")
    p.add_argument("--keep-file", help="file with one repo:glob per line; matching images are never deleted")
    p.add_argument("--no-dry-run", action="store_true", help="really delete (also needs --confirm)")
    p.add_argument("--confirm", default="", help=f'must be exactly "{CONFIRM_PHRASE}" together with --no-dry-run')
    p.add_argument("--out", default=DEFAULT_OUT, help=f"report directory (default: {DEFAULT_OUT})")
    p.add_argument("--self-test", action="store_true", help="run classification asserts and exit")
    return p.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.self_test:
        self_test()
        return 0
    try:
        check_older_than_days(args.older_than_days)
    except ValueError as exc:
        die(str(exc))

    classes = {c.strip() for c in args.classes.split(",") if c.strip()}
    bad = sorted(classes - set(DELETABLE_CLASSES))
    if bad:
        die(f"unknown or protected class(es) {bad}; choose from {', '.join(DELETABLE_CLASSES)}."
            " 'version' images are never deletable.")
    if not classes:
        die("--classes is empty")

    dry_run = not args.no_dry_run or os.environ.get("DRY_RUN", "").lower() in ("1", "true", "yes")
    if not dry_run and args.confirm != CONFIRM_PHRASE:
        die(f'refusing to delete: --no-dry-run needs --confirm "{CONFIRM_PHRASE}"')

    keep = load_keep_globs(args.keep_file) if args.keep_file else {}
    repos = resolve_repos(args.repos)

    class Opts:
        pass

    opts = Opts()
    opts.classes, opts.older_than_days, opts.keep = classes, args.older_than_days, keep

    mode = "🧪 DRY RUN" if dry_run else "🔥 REAL DELETION"
    log(f"{mode}: repos={','.join(repos)} classes={','.join(sorted(classes))} older_than_days={args.older_than_days}"
        f" keep_globs={sum(len(v) for v in keep.values())}")

    now = datetime.now(timezone.utc)
    out_dir = Path(args.out)
    records: list[dict] = []
    stats_by_repo: dict[str, dict] = {}
    for repo in repos:
        images = describe_images(repo)
        repo_records, stats = plan_repo(repo, images, opts, now)
        records += repo_records
        stats_by_repo[repo] = stats
        log(f"🔎 {repo}: {stats['scanned']} images, {stats['candidates']} candidates"
            f" ({_gb(stats['naive_bytes'])} naive GB), {stats['version']} version-protected,"
            f" {stats['skipped_keep']} kept by keep-file")

    log("")
    log(write_outputs(out_dir, records, stats_by_repo, opts, dry_run))
    if dry_run:
        log(f"🧪 DRY RUN: nothing deleted. Reports in {out_dir}/")
        return 0

    for repo in repos:
        cands = [r for r in records if r["repo"] == repo and r["action"] == "candidate"]
        if cands:
            delete_candidates(repo, cands, stats_by_repo[repo])
    log("")
    log(write_outputs(out_dir, records, stats_by_repo, opts, dry_run))
    failed = sum(s["failed"] for s in stats_by_repo.values())
    if failed:
        log(f"❌ {failed} deletions failed; see {out_dir}/candidates.csv (action=failed)")
        return 1
    log(f"✅ done. Reports in {out_dir}/")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
