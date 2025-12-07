import json
import sys
import base64
import urllib.request
import urllib.parse
import os
from pathlib import Path


def load_env_file():
    """
    Manually load .env file from project root without requiring python-dotenv.
    Parses KEY=VALUE pairs and sets them as environment variables.
    """
    script_dir = Path(__file__).parent
    project_root = script_dir.parent
    env_path = project_root / '.env'
    
    if not env_path.exists():
        return  # .env file doesn't exist, skip loading
    
    try:
        with open(env_path, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                # Skip empty lines and comments
                if not line or line.startswith('#'):
                    continue
                # Parse KEY=VALUE
                if '=' in line:
                    key, value = line.split('=', 1)
                    key = key.strip()
                    value = value.strip()
                    # Remove quotes if present
                    if value.startswith('"') and value.endswith('"'):
                        value = value[1:-1]
                    elif value.startswith("'") and value.endswith("'"):
                        value = value[1:-1]
                    # Set environment variable
                    os.environ[key] = value
    except Exception as e:
        # Silently fail if .env file can't be read
        pass


# Load .env file if it exists
load_env_file()

# --- CONFIGURATION ---

# Load from environment variables (from .env file or system environment)
JIRA_DOMAIN = os.getenv('JIRA_DOMAIN', 'https://tensorleap.atlassian.net')
JIRA_EMAIL = os.getenv('JIRA_EMAIL')
JIRA_API_TOKEN = os.getenv('JIRA_API_TOKEN')

# Project Configuration
# Set to None or empty list [] to fetch from ALL projects
# Example: PROJECTS = ["EN", "NGNB", "BF", "SR"]
PROJECTS = None  # Set to None to fetch from all projects, or specify project keys like ["EN", "NGNB"]

# JQL Configuration
JQL_STATUS = "Done"
MAX_RESULTS = 100  # Maximum number of results per request. Used for pagination.
TIMEOUT = 30  # Timeout in seconds for the request

# --- SCRIPT LOGIC ---

def fetch_all_done_tickets():
    """
    Fetches all Jira tickets with status=Done using the enhanced JQL search endpoint.
    Returns a list of all issues found.
    """
    if not all([JIRA_DOMAIN, JIRA_EMAIL, JIRA_API_TOKEN]):
        print("ERROR: Missing required configuration variables!")
        print("Please set the following environment variables or create a .env file:")
        print("  - JIRA_DOMAIN")
        print("  - JIRA_EMAIL")
        print("  - JIRA_API_TOKEN")
        print("\nYou can either:")
        print("  1. Create a .env file in the project root with these variables")
        print("  2. Set them as system environment variables")
        print("  3. Install python-dotenv: pip install python-dotenv")
        sys.exit(1)
    
    # Debug: Show loaded credentials (masked for security)
    print(f"\n--- Loaded Configuration ---")
    print(f"JIRA_DOMAIN: {JIRA_DOMAIN}")
    print(f"JIRA_EMAIL: {JIRA_EMAIL}")
    token_preview = JIRA_API_TOKEN[:20] + "..." if JIRA_API_TOKEN and len(JIRA_API_TOKEN) > 20 else "NOT SET"
    print(f"JIRA_API_TOKEN: {token_preview} (length: {len(JIRA_API_TOKEN) if JIRA_API_TOKEN else 0})")
    print()

    # Use the enhanced Jira REST API v3 JQL search endpoint (POST)
    jira_api_url = f"{JIRA_DOMAIN}/rest/api/3/search/jql"
    
    # Construct the JQL query: Find all issues with status = Done
    # Optionally filter by projects if specified
    if PROJECTS and len(PROJECTS) > 0:
        # Format the PROJECTS list into a JQL-friendly string
        project_list_jql = ", ".join(f'"{p}"' for p in PROJECTS)
        jql_query = f'project in ({project_list_jql}) AND status = "{JQL_STATUS}" ORDER BY created DESC'
    else:
        # Fetch from all projects
        jql_query = f'status = "{JQL_STATUS}" ORDER BY created DESC'

    print(f"--- Running JQL Query: {jql_query} ---")
    print(f"API Endpoint: {jira_api_url}")
    print("Fetching data from Jira...")

    # Prepare Basic Authentication Header
    auth_string = f"{JIRA_EMAIL}:{JIRA_API_TOKEN}"
    encoded_auth = base64.b64encode(auth_string.encode('utf-8')).decode('utf-8')
    
    headers = {
        "Accept": "application/json",
        "Content-Type": "application/json",
        "Authorization": f"Basic {encoded_auth}"
    }
    
    next_page_token = None
    all_issues = []
    page_count = 0
    
    # Loop to handle pagination using nextPageToken
    while True:
        # Build request body according to API documentation
        request_body = {
            "jql": jql_query,
            "fields": ["summary", "key", "status", "created", "updated", "project"],
            "maxResults": MAX_RESULTS
        }
        
        # Add nextPageToken if we have one (for pagination)
        if next_page_token:
            request_body["nextPageToken"] = next_page_token

        try:
            # Convert request body to JSON
            json_data = json.dumps(request_body).encode('utf-8')
            
            # Print request details
            print(f"\n--- API Request (Page {page_count + 1}) ---")
            print(f"URL: {jira_api_url}")
            print(f"Method: POST")
            print(f"Headers: {json.dumps({k: v if k != 'Authorization' else 'Basic ***' for k, v in headers.items()}, indent=2)}")
            print(f"Request Body: {json.dumps(request_body, indent=2)}")
            
            # Create the POST request object
            req = urllib.request.Request(jira_api_url, data=json_data, headers=headers, method='POST')
            
            # Variables to store outside the with block
            status_code = None
            login_reason = ''
            data = {}
            
            # Open the URL and get the response
            with urllib.request.urlopen(req, timeout=TIMEOUT) as response:
                # Read the response body
                response_body = response.read().decode('utf-8')
                status_code = response.getcode()
                response_headers = dict(response.headers)
                login_reason = response_headers.get('X-Seraph-Loginreason', '')
                
                # Print response details
                print(f"\n--- API Response (Page {page_count + 1}) ---")
                print(f"Status Code: {status_code}")
                print(f"Status: {response.msg}")
                print(f"Response Headers: {response_headers}")
                
                # Parse JSON response
                data = json.loads(response_body)
                
                # Print response body (formatted JSON)
                print(f"Response Body: {json.dumps(data, indent=2)}")
                
                # Check for authentication failure in headers (Jira-specific)
                if login_reason == 'AUTHENTICATED_FAILED':
                    print(f"\n❌ AUTHENTICATION FAILED!")
                    print(f"   The 'X-Seraph-Loginreason: AUTHENTICATED_FAILED' header indicates authentication failed.")
                    print(f"   Even though you got 200 OK, your credentials are not valid.")
                    print(f"\n   Please check:")
                    print(f"   1. JIRA_EMAIL in .env file is correct")
                    print(f"   2. JIRA_API_TOKEN in .env file is valid and not expired")
                    print(f"   3. The API token has proper permissions")
                    print(f"\n   To create a new API token:")
                    print(f"   https://id.atlassian.com/manage-profile/security/api-tokens")
                    print(f"\n   Exiting due to authentication failure.")
                    sys.exit(1)
                
                # Check for potential permission issues or warnings
                if status_code == 200:
                    # Check for warning messages in response
                    warning_messages = data.get("warningMessages", [])
                    if warning_messages:
                        print(f"\n⚠️  WARNING: API returned warning messages:")
                        for warning in warning_messages:
                            print(f"   - {warning}")

            issues = data.get("issues", [])
            is_last = data.get("isLast", True)
            next_page_token = data.get("nextPageToken")
            
            # Check if response indicates potential permission issues (only on first page)
            if status_code == 200 and len(issues) == 0 and page_count == 0 and login_reason != 'AUTHENTICATED_FAILED':
                print(f"\n⚠️  NOTE: Got 200 OK but no issues returned on first page.")
                print(f"   This could indicate:")
                print(f"   - No matching issues exist")
                print(f"   - Permission issues (you may not have access to matching issues)")
                print(f"   - JQL query might need adjustment")

            all_issues.extend(issues)
            page_count += 1
            print(f"  > Fetched page {page_count}: {len(issues)} issues (total: {len(all_issues)})...")

            # Check if this is the last page
            if is_last or not next_page_token:
                print(f"  > Reached end of results (isLast: {is_last})")
                break

            if not issues:
                break  # Stop if no issues are returned (end of data)
                
        except urllib.error.HTTPError as e:
            # Handle Jira/API errors (e.g., 401 Unauthorized, 404 Not Found)
            print(f"\nERROR: HTTP request failed. Status Code: {e.code}")
            print(f"URL: {jira_api_url}")
            print(f"Request Body: {json.dumps(request_body, indent=2)}")
            try:
                # Try to read and display the error response body
                error_body = e.read().decode('utf-8')
                print(f"Response Body: {error_body}")
                # Try to parse as JSON for better error message
                try:
                    error_json = json.loads(error_body)
                    if 'errorMessages' in error_json:
                        print(f"Error Messages: {error_json['errorMessages']}")
                    if 'errors' in error_json:
                        print(f"Errors: {error_json['errors']}")
                except:
                    pass
            except Exception:
                pass
            print("\nPossible causes:")
            print("  - Incorrect domain or API endpoint")
            print("  - Invalid token/email authentication")
            print("  - JQL query syntax error")
            sys.exit(1)
        except urllib.error.URLError as e:
            # Handle network/connection errors (e.g., DNS, timeout)
            print(f"\nERROR: A URL or connection error occurred: {e.reason}")
            sys.exit(1)
        except Exception as e:
            # Catch any other unexpected errors (like JSON decoding errors)
            print(f"\nAn unexpected error occurred: {e}")
            sys.exit(1)
    
    # Print summary
    print(f"\n--- Query Summary ---")
    print(f"Total issues found: {len(all_issues)}")
    print(f"JQL Query used: {jql_query}")
    print(f"Status filter: {JQL_STATUS}")
    if PROJECTS and len(PROJECTS) > 0:
        print(f"Projects filtered: {', '.join(PROJECTS)}")
    else:
        print("Projects filtered: ALL projects")
    
    if len(all_issues) == 0:
        print("\nNo tickets found with status='Done'")
        print("\nPossible reasons:")
        print("  1. No tickets in 'Done' status")
        if PROJECTS and len(PROJECTS) > 0:
            print(f"  2. No tickets in specified projects: {', '.join(PROJECTS)}")
        print("  3. Status name might be different (e.g., 'DONE', 'Closed', etc.)")
        print("  4. Authentication or permission issues")
    
    return all_issues


def print_tickets_summary(issues):
    """
    Prints a summary of all fetched tickets.
    """
    if not issues:
        print("\nNo tickets to display.")
        return
    
    print(f"\n--- Tickets Summary ({len(issues)} total) ---\n")
    
    for issue in issues:
        key = issue.get('key', 'N/A')
        fields = issue.get('fields', {})
        summary = fields.get('summary', 'N/A')
        project = fields.get('project', {})
        project_key = project.get('key', 'N/A') if project else 'N/A'
        created = fields.get('created', 'N/A')
        
        print(f"{key} [{project_key}]: {summary}")
        if created != 'N/A':
            print(f"  Created: {created}")


def save_tickets_to_json(issues, output_file="done_tickets.json"):
    """
    Saves all fetched tickets to a JSON file.
    """
    script_dir = os.path.dirname(os.path.dirname(__file__))
    output_path = os.path.join(script_dir, output_file)
    
    try:
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(issues, f, indent=2, ensure_ascii=False)
        print(f"\nTickets saved to: {output_path}")
    except Exception as e:
        print(f"\nError saving tickets to file: {e}")


if __name__ == "__main__":
    # Fetch all Done tickets
    tickets = fetch_all_done_tickets()
    
    # Print summary
    print_tickets_summary(tickets)
    
    # Save to JSON file
    save_tickets_to_json(tickets)
    
    print(f"\n✓ Completed! Found {len(tickets)} tickets with status='Done'")

