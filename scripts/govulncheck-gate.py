#!/usr/bin/env python3
"""Judge a govulncheck JSON report against a list of accepted findings.

govulncheck has no way to accept a finding, and it exits 3 for "your code
calls a vulnerable symbol" whether or not a fixed version exists. When an
advisory lands against a dependency with no patched release, that leaves two
bad options: turn the scan off, or leave every build red. This gate is the
third: accept named advisories, with a written reason, and refuse to let the
acceptance drift out of date.

Only findings that reach a function we call are judged, which is the same set
govulncheck's own "Your code is affected by N vulnerabilities" counts. A
vulnerability in a module we require but never call is reported by govulncheck
and ignored here, exactly as govulncheck already treats it.

It exits 1 when:
  1. A called finding is not listed in the allow file.
  2. A listed advisory now names a fixed version. Upgrade instead of accepting.
  3. A listed advisory is no longer reported. The line is stale.

The last two are what stop an acceptance from outliving its reason.

Allow-file format: one advisory id, whitespace, then the reason it is
accepted. Lines starting with `#` are comments.

Usage: govulncheck-gate.py PATH_TO_REPORT [PATH_TO_ALLOW_FILE]
where PATH_TO_REPORT holds the output of `govulncheck -format json ./...`.
"""

import json
import sys

DEFAULT_ALLOW_FILE = ".govulncheck-allow.txt"


def load_report(path):
    """Read the message stream govulncheck writes.

    The -format json output is a series of concatenated JSON objects rather
    than an array, so it is decoded one value at a time.
    """
    decoder = json.JSONDecoder()
    raw = open(path, encoding="utf-8").read()
    messages = []
    index = 0
    while index < len(raw):
        while index < len(raw) and raw[index].isspace():
            index += 1
        if index >= len(raw):
            break
        message, index = decoder.raw_decode(raw, index)
        messages.append(message)
    return messages


def called_advisories(messages):
    """Advisory ids whose vulnerable code this module actually calls.

    A finding whose first trace frame names a function is reachable from our
    own code. A finding that stops at the module level is a dependency we
    require but never call.
    """
    return {
        finding["osv"]
        for finding in (m["finding"] for m in messages if "finding" in m)
        if finding.get("trace") and finding["trace"][0].get("function")
    }


def load_allow_file(path):
    """Advisory id to the stated reason it is accepted."""
    accepted = {}
    try:
        handle = open(path, encoding="utf-8")
    except FileNotFoundError:
        return accepted
    with handle:
        for line in handle:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            advisory_id, _, reason = line.partition(" ")
            accepted[advisory_id] = reason.strip()
    return accepted


def fixed_versions(advisory):
    """Versions the advisory names as carrying the fix, if any."""
    versions = set()
    for affected in advisory.get("affected", []):
        for span in affected.get("ranges", []):
            for event in span.get("events", []):
                if "fixed" in event:
                    versions.add(event["fixed"])
    return sorted(versions)


def check(messages, accepted, allow_path):
    """Return the failure lines, empty when the report is acceptable."""
    advisories = {m["osv"]["id"]: m["osv"] for m in messages if "osv" in m}
    called = called_advisories(messages)
    failures = []

    def summary(advisory_id):
        return advisories.get(advisory_id, {}).get("summary", "(no summary)")

    for advisory_id in sorted(called - set(accepted)):
        failures.append(
            f"  {advisory_id}  NOT ACCEPTED: {summary(advisory_id)}\n"
            f"      https://pkg.go.dev/vuln/{advisory_id}\n"
            f"      Upgrade the module, or add it to {allow_path} with a reason."
        )

    for advisory_id in sorted(accepted):
        if advisory_id not in called:
            failures.append(
                f"  {advisory_id}  STALE: no longer reported.\n"
                f"      Remove the line from {allow_path}."
            )
            continue
        fixes = fixed_versions(advisories.get(advisory_id, {}))
        if fixes:
            failures.append(
                f"  {advisory_id}  FIX AVAILABLE in {', '.join(fixes)}: {summary(advisory_id)}\n"
                f"      It was accepted because no fix existed. Upgrade and remove the line."
            )

    return failures, called, advisories


def main():
    if len(sys.argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    report_path = sys.argv[1]
    allow_path = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_ALLOW_FILE

    messages = load_report(report_path)
    accepted = load_allow_file(allow_path)
    failures, called, advisories = check(messages, accepted, allow_path)

    for advisory_id in sorted(called & set(accepted)):
        summary = advisories.get(advisory_id, {}).get("summary", "(no summary)")
        print(f"  accepted: {advisory_id}  {summary}")

    if failures:
        print("\ngovulncheck gate FAILED:\n")
        print("\n".join(failures))
        return 1

    print(f"govulncheck: {len(called)} called finding(s), all accepted in {allow_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
