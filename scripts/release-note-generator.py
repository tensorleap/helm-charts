from jira import JIRA
import json
import os
from pathlib import Path

# Load .env file if it exists
def load_env_file():
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

load_env_file()

# Get credentials from environment
JIRA_DOMAIN = os.getenv('JIRA_DOMAIN', 'https://tensorleap.atlassian.net')
JIRA_EMAIL = os.getenv('JIRA_EMAIL', 'omri.yonatani@tensorleap.ai')
JIRA_API_TOKEN = os.getenv('JIRA_API_TOKEN', '')

if not JIRA_API_TOKEN:
    print("ERROR: JIRA_API_TOKEN not set. Please set it in .env file or as environment variable.")
    exit(1)

# Connect to Jira using SDK - much simpler!
jira = JIRA(
    server=JIRA_DOMAIN,
    basic_auth=(JIRA_EMAIL, JIRA_API_TOKEN)
)

print(f"✅ Connected to Jira: {JIRA_DOMAIN}")
print(f"User: {jira.current_user()}\n")

# Search for issues using JQL - SDK handles all the HTTP requests for you!
jql_query = 'project = EN and status = Done'

print(f"Searching with JQL: {jql_query}\n")

# The SDK makes it super easy - just call search_issues()
issues = jira.search_issues(
    jql_query,
    maxResults=100,
    fields=['summary', 'key', 'status', 'created', 'updated', 'project']
)

print(f"Found {len(issues)} issues:\n")

# Process the issues - SDK returns Issue objects, not raw JSON
results = []
for issue in issues:
    issue_data = {
        'key': issue.key,
        'summary': issue.fields.summary,
        'status': issue.fields.status.name,
        'created': issue.fields.created,
        'updated': issue.fields.updated,
        'project': issue.fields.project.key
    }
    results.append(issue_data)
    print(f"{issue.key}: {issue.fields.summary}")

# Print as JSON
print(f"\n\nJSON Output:")
print(json.dumps(results, sort_keys=True, indent=4, separators=(",", ": ")))

