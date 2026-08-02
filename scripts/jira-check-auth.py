"""Diagnose Jira credentials from .env.

Run with `make jira-check`. Reports the shape of JIRA_API_TOKEN (never the
value) and the raw response from /myself, so a bad paste can be told apart
from a bad token.
"""
import os
import requests
from pathlib import Path

MYSELF_PATH = "/rest/api/3/myself"


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


def describe(name: str, value: str) -> None:
    """Print the shape of a secret without printing the secret."""
    dirty = [c for c in value if ord(c) < 32 or ord(c) > 126]
    print(f"  {name}:")
    print(f"    length         : {len(value)}")
    print(f"    starts with    : {value[:6] if value else '(empty)'}")
    print(f"    inner spaces   : {'YES  <-- bad paste' if ' ' in value.strip() else 'no'}")
    print(f"    control/unicode: {'YES  <-- bad paste' if dirty else 'no'}")


def main():
    load_env_file()

    domain = os.getenv('JIRA_DOMAIN', 'https://tensorleap.atlassian.net').rstrip('/')
    email = os.getenv('JIRA_EMAIL', 'omri.yonatani@tensorleap.ai')
    token = os.getenv('JIRA_API_TOKEN', '')

    print("Credentials loaded from .env")
    print(f"  JIRA_DOMAIN: {domain}")
    print(f"  JIRA_EMAIL : {email}")

    if not token:
        print("\n❌ JIRA_API_TOKEN is empty. Add it to .env as:")
        print("   JIRA_API_TOKEN=ATATT...   (no quotes, no trailing spaces)")
        exit(1)

    describe("JIRA_API_TOKEN", token)

    if not token.startswith("ATATT"):
        print("\n⚠️  Classic Atlassian tokens start with 'ATATT'. This may be the")
        print("    wrong value, a truncated copy, or a scoped token.")

    print(f"\nGET {domain}{MYSELF_PATH}")
    try:
        r = requests.get(
            f"{domain}{MYSELF_PATH}",
            auth=(email, token),
            headers={"Accept": "application/json"},
            timeout=30,
        )
    except requests.RequestException as e:
        print(f"❌ Request failed: {e}")
        exit(1)

    print(f"Status: {r.status_code}")

    if r.status_code == 200:
        who = r.json()
        print(f"\n✅ Authenticated as: {who.get('displayName')} <{who.get('emailAddress')}>")
        print("   This token is good. Set the same value as the GitHub secret:")
        print("   gh secret set JIRA_API_TOKEN --repo tensorleap/helm-charts")
        return

    print(f"Body: {r.text[:600]}")

    if r.status_code == 400:
        print("\n❌ 400 — Atlassian rejected the request itself, not the credentials.")
        print("   Usually junk in the token value. Check the shape report above.")
    elif r.status_code == 401:
        print("\n❌ 401 — credentials rejected. The token is revoked/expired, belongs")
        print("   to a different account than JIRA_EMAIL, or is a scoped token")
        print("   (scoped tokens only work via https://api.atlassian.com/ex/jira/<cloudId>).")
    elif r.status_code == 403:
        print("\n❌ 403 — authenticated, but this account lacks permission.")
    exit(1)


if __name__ == "__main__":
    main()
