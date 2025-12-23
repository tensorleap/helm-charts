from jira import JIRA
from jira.resources import Issue
import os
import re
from pathlib import Path
from datetime import datetime
from collections import defaultdict
from typing import List, Optional

# =============================================================================
# Configuration
# =============================================================================

# Projects to include in release notes
# PROJECTS = ["EN", "NGNB", "BF", "SR"]
PROJECTS = ["EN"]

FIX_VERSION_JIRA_FIELD= "fixVersion"

# JQL query for finding done tickets without a fix version
JQL_TEMPLATE = f'project in ({", ".join(PROJECTS)}) AND status = Done AND {FIX_VERSION_JIRA_FIELD} IS EMPTY ORDER BY issuetype ASC'

# Output file path
OUTPUT_FILE = "RELEASE_NOTES.md"



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


def get_chart_version() -> str:
    """Read chart version from Chart.yaml."""
    script_dir = Path(__file__).parent
    chart_path = script_dir.parent / "charts" / "tensorleap" / "Chart.yaml"
    
    try:
        with open(chart_path, 'r') as f:
            for line in f:
                match = re.match(r'^version:\s*(.+)$', line.strip())
                if match:
                    return match.group(1).strip()
    except Exception:
        pass
    return "unknown"


def categorize_issue_type(issue_type: str) -> str:
    """Map Jira issue types to release note categories."""
    type_lower = issue_type.lower()
    
    if 'bug' in type_lower:
        return '🐛 Bug Fixes'
    if any(t in type_lower for t in ['story', 'task', 'improvement', 'feature', 'enhancement']):
        return '✨ New Features & Improvements'
    return '📝 Other'


def create_fix_version(jira: JIRA, project_key: str, version_name: str) -> bool:
    """Create a FixVersion in Jira project. Returns True if created or already exists."""
    print(f"\n📌 Creating Jira FixVersion '{version_name}' in project {project_key}...")
    
    try:
        jira.create_version(name=version_name, project=project_key)
        print(f"  ✅ FixVersion '{version_name}' created successfully")
        return True
    except Exception as e:
        error_msg = str(e).lower()
        # Version already exists is not an error
        if 'already exists' in error_msg or 'duplicate' in error_msg:
            print(f"  ✅ FixVersion '{version_name}' already exists")
            return True
        print(f"  ❌ Failed to create FixVersion: {e}")
        return False


def generate_release_notes(issues: List[Issue], version: str, jira_domain: str) -> str:
    """Generate markdown release notes."""
    # Group issues by category
    categories = defaultdict(list)
    
    for issue in issues:
        issue_type = issue.fields.issuetype.name
        category = categorize_issue_type(issue_type)
        categories[category].append(issue)
    
    # Build markdown
    lines = []
    lines.append(f"# Release Notes - v{version}")
    lines.append(f"")
    lines.append(f"**Release Date:** {datetime.now().strftime('%Y-%m-%d')}")
    lines.append(f"**Total Changes:** {len(issues)}")
    lines.append(f"")
    lines.append("---")
    lines.append("")
    
    # Define category order
    category_order = [
        '✨ New Features & Improvements',
        '🐛 Bug Fixes',
        '📝 Other'
    ]
    
    # Output each category
    for category in category_order:
        if category in categories:
            lines.append(f"## {category}")
            lines.append("")
            for issue in categories[category]:
                ticket_url = f"{jira_domain}/browse/{issue.key}"
                lines.append(f"- [{issue.key}]({ticket_url}): {issue.fields.summary}")
            lines.append("")
    
    return "\n".join(lines)


# =============================================================================
# Main
# =============================================================================

def main():
    load_env_file()
    
    # Get credentials
    JIRA_DOMAIN = os.getenv('JIRA_DOMAIN', 'https://tensorleap.atlassian.net')
    JIRA_EMAIL = os.getenv('JIRA_EMAIL', 'omri.yonatani@tensorleap.ai')
    JIRA_API_TOKEN = os.getenv('JIRA_API_TOKEN', '')
    
    if not JIRA_API_TOKEN:
        print("ERROR: JIRA_API_TOKEN not set.")
        print("Please set it in .env file or as environment variable.")
        exit(1)
    
    # Connect to Jira
    print(f"Connecting to Jira: {JIRA_DOMAIN}")
    jira = JIRA(
        server=JIRA_DOMAIN,
        basic_auth=(JIRA_EMAIL, JIRA_API_TOKEN)
    )
    print(f"✅ Connected as: {jira.current_user()}\n")
    
    # Get chart version
    version = get_chart_version()
    print(f"📦 Chart version: {version}\n")
    
    # Create FixVersion in Jira (before fetching issues)
    create_fix_version_enabled = os.getenv('CREATE_FIX_VERSION', 'false').lower() == 'true'
    if create_fix_version_enabled:
        for project in PROJECTS:
            create_fix_version(jira, project, version)
        print()
    
    # Build JQL query
    project_list = ", ".join(PROJECTS)
    jql_query = JQL_TEMPLATE.format(projects=project_list)
    print(f"🔍 JQL: {jql_query}\n")
    
    # Fetch issues (with pagination)
    all_issues = []
    start_at = 0
    max_results = 100
    
    while True:
        issues = jira.search_issues(
            jql_query,
            startAt=start_at,
            maxResults=max_results,
            fields=['summary', 'issuetype', 'project']
        )
        
        all_issues.extend(issues)
        print(f"  Fetched {len(all_issues)} issues...")
        
        if len(issues) < max_results:
            break
        start_at += max_results
    
    print(f"\n📊 Total issues found: {len(all_issues)}\n")
    
    if not all_issues:
        print("No issues found matching the criteria.")
        return
    
    # Generate release notes
    release_notes = generate_release_notes(all_issues, version, JIRA_DOMAIN)
    
    # Print to console
    print("=" * 60)
    print(release_notes)
    print("=" * 60)
    
    # Save to file (prepend to existing content so newest is at top)
    script_dir = Path(__file__).parent
    output_path = script_dir.parent / OUTPUT_FILE
    
    existing_content = ""
    if output_path.exists():
        with open(output_path, 'r', encoding='utf-8') as f:
            existing_content = f.read()
    
    # Prepend new release notes with a separator
    separator = "\n\n---\n\n"
    if existing_content:
        new_content = release_notes + separator + existing_content
    else:
        new_content = release_notes
    
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(new_content)
    
    print(f"\n✅ Release notes saved to: {output_path}")
    
    # Update fixVersion on all issues
    update_fix_version = os.getenv('UPDATE_FIX_VERSION', 'false').lower() == 'true'
    
    if update_fix_version:
        print(f"\n🔄 Updating fixVersion to '{version}' on {len(all_issues)} issues...")
        
        success_count = 0
        fail_count = 0
        
        for issue in all_issues:
            try:
                issue.update(fields={'fixVersions': [{'name': version}]})
                print(f"  ✅ {issue.key}")
                success_count += 1
            except Exception as e:
                print(f"  ❌ {issue.key}: {e}")
                fail_count += 1
        
        print(f"\n📊 fixVersion update complete: {success_count} succeeded, {fail_count} failed")
    else:
        print(f"\n⏭️  Skipping fixVersion update (set UPDATE_FIX_VERSION=true to enable)")


if __name__ == "__main__":
    main()

