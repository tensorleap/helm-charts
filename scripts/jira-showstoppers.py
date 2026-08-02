from jira import JIRA
from jira.resources import Issue
import json
import os
from pathlib import Path
from datetime import datetime, timezone
from urllib.parse import quote
from typing import List, Optional

# =============================================================================
# Configuration
# =============================================================================

# Label that marks a bug as a showstopper
SHOWSTOPPER_LABEL = "showstopper"

# Priority that qualifies as a showstopper
SHOWSTOPPER_PRIORITY = "Highest"

# Projects to scan. Empty list = all projects the account can see.
# Override with SHOWSTOPPER_PROJECTS="EN,NGNB,BF,SR".
PROJECTS: List[str] = ["BF"]

# Slack channel the daily message goes to. Override with SLACK_CHANNEL_ID.
DEFAULT_SLACK_CHANNEL = "C05KWMJ0EJE"

# Slack renders at most 50 blocks per message; each issue costs 2.
MAX_ISSUES_IN_MESSAGE = 20

# Slack payload written for the GitHub Action to post
OUTPUT_FILE = "slack-showstoppers.json"


# =============================================================================
# Helper Functions
# =============================================================================

def load_env_file():
    """Load .env file from project root."""
    script_dir = Path(__file__).parent
    project_root = script_dir.parent
    env_path = project_root / '.env'

    if not env_path.exists():
        return

    try:
        with open(env_path, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith('#'):
                    continue
                if '=' in line:
                    key, value = line.split('=', 1)
                    key = key.strip()
                    value = value.strip()
                    if value.startswith('"') and value.endswith('"'):
                        value = value[1:-1]
                    elif value.startswith("'") and value.endswith("'"):
                        value = value[1:-1]
                    os.environ[key] = value
    except Exception:
        pass


def build_jql(projects: List[str]) -> str:
    """JQL for open (not Done) highest-priority showstopper bugs."""
    clauses = [
        f'priority = {SHOWSTOPPER_PRIORITY}',
        f'labels = {SHOWSTOPPER_LABEL}',
        'statusCategory != Done',
    ]
    if projects:
        clauses.insert(0, f'project in ({", ".join(projects)})')
    return ' AND '.join(clauses) + ' ORDER BY created ASC'


def age_in_days(created: Optional[str]) -> Optional[int]:
    """Days since the issue was created, from a Jira timestamp."""
    if not created:
        return None
    try:
        # Jira format: 2026-07-30T12:34:56.000+0300
        dt = datetime.strptime(created, '%Y-%m-%dT%H:%M:%S.%f%z')
    except (TypeError, ValueError):
        return None
    return (datetime.now(timezone.utc) - dt).days


def assignee_name(issue: Issue) -> str:
    assignee = getattr(issue.fields, 'assignee', None)
    if assignee is None:
        return "Unassigned"
    return getattr(assignee, 'displayName', None) or str(assignee)


def status_name(issue: Issue) -> str:
    status = getattr(issue.fields, 'status', None)
    return getattr(status, 'name', None) or "Unknown"


def truncate(text: str, limit: int = 160) -> str:
    text = " ".join((text or "").split())
    return text if len(text) <= limit else text[: limit - 1] + "…"


def build_slack_payload(issues: List[Issue], jira_domain: str, jql: str,
                        channel: str) -> dict:
    """Build a chat.postMessage payload with the showstopper table."""
    today = datetime.now().strftime('%Y-%m-%d')
    jql_url = f"{jira_domain}/issues/?jql={quote(jql)}"

    if not issues:
        fallback = "There are no showstoppers champ! 😃"
        blocks = [
            {
                "type": "header",
                "text": {
                    "type": "plain_text",
                    "text": "🛑 Open Showstoppers",
                    "emoji": True,
                },
            },
            {
                "type": "section",
                "text": {
                    "type": "mrkdwn",
                    "text": "There are no showstoppers champ! 😃",
                },
            },
            {
                "type": "context",
                "elements": [
                    {
                        "type": "mrkdwn",
                        "text": f"{today} · <{jql_url}|Jira query>",
                    }
                ],
            },
        ]
        return {"channel": channel, "text": fallback, "blocks": blocks}

    count = len(issues)
    fallback = f"🛑 {count} open showstopper{'s' if count != 1 else ''}"
    blocks = [
        {
            "type": "header",
            "text": {
                "type": "plain_text",
                "text": f"🛑 Open Showstoppers ({count})",
                "emoji": True,
            },
        },
        {
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": (
                        f"{today} · priority *{SHOWSTOPPER_PRIORITY}* · "
                        f"label *{SHOWSTOPPER_LABEL}* · <{jql_url}|open in Jira>"
                    ),
                }
            ],
        },
        {"type": "divider"},
    ]

    for issue in issues[:MAX_ISSUES_IN_MESSAGE]:
        url = f"{jira_domain}/browse/{issue.key}"
        age = age_in_days(getattr(issue.fields, 'created', None))
        age_text = f"{age}d old" if age is not None else "age unknown"
        blocks.append({
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": f"*<{url}|{issue.key}>* — {truncate(issue.fields.summary)}",
            },
        })
        blocks.append({
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": (
                        f"*Status:* {status_name(issue)}   "
                        f"*Assignee:* {assignee_name(issue)}   "
                        f"*Age:* {age_text}"
                    ),
                }
            ],
        })

    remaining = count - MAX_ISSUES_IN_MESSAGE
    if remaining > 0:
        blocks.append({
            "type": "context",
            "elements": [
                {
                    "type": "mrkdwn",
                    "text": f"…and {remaining} more — <{jql_url}|see all in Jira>",
                }
            ],
        })

    return {"channel": channel, "text": fallback, "blocks": blocks}


def print_preview(issues: List[Issue]) -> None:
    """Plain-text table for local runs / CI logs."""
    if not issues:
        print("There are no showstoppers champ! 😃")
        return

    rows = []
    for issue in issues:
        age = age_in_days(getattr(issue.fields, 'created', None))
        rows.append((
            issue.key,
            truncate(issue.fields.summary, 60),
            status_name(issue),
            assignee_name(issue),
            f"{age}d" if age is not None else "?",
        ))

    headers = ("KEY", "SUMMARY", "STATUS", "ASSIGNEE", "AGE")
    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]
    line = "  ".join(h.ljust(widths[i]) for i, h in enumerate(headers))
    print(line)
    print("-" * len(line))
    for row in rows:
        print("  ".join(cell.ljust(widths[i]) for i, cell in enumerate(row)))


# =============================================================================
# Main
# =============================================================================

def main():
    load_env_file()

    JIRA_DOMAIN = os.getenv('JIRA_DOMAIN', 'https://tensorleap.atlassian.net').rstrip('/')
    JIRA_EMAIL = os.getenv('JIRA_EMAIL', 'omri.yonatani@tensorleap.ai')
    JIRA_API_TOKEN = os.getenv('JIRA_API_TOKEN', '')
    SLACK_CHANNEL_ID = os.getenv('SLACK_CHANNEL_ID', DEFAULT_SLACK_CHANNEL)

    if not JIRA_API_TOKEN:
        print("ERROR: JIRA_API_TOKEN not set.")
        print("Please set it in .env file or as environment variable.")
        exit(1)

    if not SLACK_CHANNEL_ID:
        print("ERROR: SLACK_CHANNEL_ID not set.")
        print("Set it to the Slack channel the daily message should go to.")
        exit(1)

    projects_env = os.getenv('SHOWSTOPPER_PROJECTS', '')
    projects = [p.strip() for p in projects_env.split(',') if p.strip()] or PROJECTS

    print(f"Connecting to Jira: {JIRA_DOMAIN}")
    jira = JIRA(
        server=JIRA_DOMAIN,
        basic_auth=(JIRA_EMAIL, JIRA_API_TOKEN)
    )
    print(f"✅ Connected as: {jira.current_user()}\n")

    jql_query = build_jql(projects)
    print(f"🔍 JQL: {jql_query}\n")

    issues = list(jira.enhanced_search_issues(
        jql_query,
        fields=['summary', 'status', 'assignee', 'created', 'priority', 'labels']
    ))

    print(f"📊 Open showstoppers: {len(issues)}\n")
    print_preview(issues)

    payload = build_slack_payload(issues, JIRA_DOMAIN, jql_query, SLACK_CHANNEL_ID)

    output_path = Path(__file__).parent.parent / OUTPUT_FILE
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)

    print(f"\n✅ Slack payload written to: {output_path}")


if __name__ == "__main__":
    main()
