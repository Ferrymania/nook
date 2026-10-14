#!/usr/bin/env python3
"""Generate categorized release notes from commits since the previous release."""

import argparse
import os
import re
import subprocess
from urllib.parse import quote


CATEGORIES = {
    "feat": "Features · 新功能",
    "fix": "Fixes · 修复",
    "perf": "Performance · 性能",
    "refactor": "Refactor · 重构",
    "docs": "Docs · 文档与配置",
    "build": "Build / CI · 构建与发布",
    "ci": "Build / CI · 构建与发布",
}
BREAKING = "Breaking Changes · 不兼容变更"
OTHER = "Other Changes · 其他变更"
CONVENTIONAL = re.compile(r"^([a-z]+)(?:\(([^)]+)\))?(!)?:\s*(.+)$")


def markdown_text(text):
    return re.sub(r"([\\`*_{}\[\]<>])", r"\\\1", text)


def release_notes(tag, ref, repository):
    previous = subprocess.run(
        [
            "git", "describe", "--tags", "--abbrev=0",
            "--match=nook-v*", "--match=mebox-v*", f"--exclude={tag}", ref,
        ],
        text=True, capture_output=True,
    )
    base = previous.stdout.strip() if previous.returncode == 0 else ""
    commit_range = f"{base}..{ref}" if base else ref
    log = subprocess.check_output(
        ["git", "log", "--no-merges", "--format=%H%x00%s", commit_range],
        text=True,
    )
    groups = {name: [] for name in [BREAKING, *dict.fromkeys(CATEGORIES.values()), OTHER]}
    repo_url = f"https://github.com/{repository}"
    for line in log.splitlines():
        commit, subject = line.split("\0", 1)
        match = CONVENTIONAL.match(subject)
        category = OTHER
        description = subject
        if match:
            kind, scope, breaking, description = match.groups()
            category = BREAKING if breaking else CATEGORIES.get(kind, OTHER)
            if scope:
                description = f"{scope}: {description}"
        groups[category].append(
            f"- {markdown_text(description)} ([{commit[:7]}]({repo_url}/commit/{commit}))"
        )

    lines = ["## Changelog", ""]
    for category, entries in groups.items():
        if entries:
            lines.extend([f"### {category}", "", *entries, ""])
    if not log.strip():
        lines.extend(["本次重新构建，代码与上一版本一致。", ""])
    if base:
        compare = f"{quote(base, safe='')}...{quote(tag, safe='')}"
        lines.append(f"**Full Changelog**: [{base}...{tag}]({repo_url}/compare/{compare})")
    else:
        lines.append(f"**Full Changelog**: [全部提交]({repo_url}/commits/{quote(tag, safe='')})")
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True, help="Release tag being published")
    parser.add_argument("--ref", default="HEAD", help="Commit to publish (default: HEAD)")
    parser.add_argument(
        "--repository", default=os.environ.get("GITHUB_REPOSITORY", "Ferrymania/nook"),
        help="GitHub owner/repository used for links",
    )
    args = parser.parse_args()
    print(release_notes(args.tag, args.ref, args.repository), end="")
