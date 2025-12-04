import json
import sys
import base64
import urllib.request
import urllib.parse
import ssl
import os
import re
from datetime import datetime

# --- CONFIGURATION ---

# 1. IMPORTANT: Replace 'YOUR_JIRA_DOMAIN' with your actual Jira Cloud URL
#    (e.g., mycompany.atlassian.net)
JIRA_DOMAIN = "https://tensorleap.atlassian.net"

# 2. IMPORTANT: Replace 'YOUR_EMAIL' with the email associated with the API Token.
#    Jira uses Basic Auth with (Email, API Token)
JIRA_EMAIL = "asaf.yehezkel@tensorleap.ai"

# 3. The API Token provided by the user. Used for authentication.
JIRA_API_TOKEN = "ATATT3xFfGF0poI8Mri19YIXs0p2WwspYyyoo0gbnCVXoJqQ8ic0N-DXHURcbuFUDeLX8wk6gyDO6L6YNJYS1I6KEkyFKupBJSKuBg3QBsUnG17ud4AOXU7wbgWHFdamdJ5q0qUF7QgLDotqneZ0ml9gEnWghsLchGv1QKCg9GT4hqlpt1M=55EE7F6C"

# 4. IMPORTANT: Update this list with the Jira project keys you want to query (e.g., 'PROJ1', 'PROJ2').
#    Based on your screenshot, the "Engineering" project key appears to be 'EN'.
#    Add other project keys (like 'BFR' for 'Bugs & FR') if needed.
PROJECTS = ["EN"]

# JQL Configuration
JQL_STATUS = "Done"
MAX_RESULTS = 100 # Maximum number of results per request. Used for pagination.
TIMEOUT = 30 # Timeout in seconds for the request

# --- SCRIPT LOGIC ---

def get_chart_version():
    """
    Reads the chart version from charts/tensorleap/Chart.yaml
    """
    chart_yaml_path = os.path.join(os.path.dirname(os.path.dirname(__file__)), "charts", "tensorleap", "Chart.yaml")
    try:
        with open(chart_yaml_path, 'r') as f:
            for line in f:
                # Match "version: X.Y.Z" pattern
                match = re.match(r'^version:\s*(.+)$', line.strip())
                if match:
                    return match.group(1).strip()
        return 'unknown'
    except Exception as e:
        print(f"Warning: Could not read chart version from {chart_yaml_path}: {e}")
        return 'unknown'

def generate_release_notes():
    """
    Connects to Jira using the standard urllib library, retrieves all tickets in
    the specified projects that are in the 'Done' status, and formats their
    summaries as a release note. Handles pagination automatically.
    Writes the release notes to a file in the charts directory.
    """
    if not all([JIRA_DOMAIN, JIRA_EMAIL, JIRA_API_TOKEN, PROJECTS]):
        print("ERROR: Please update all configuration variables (JIRA_DOMAIN, JIRA_EMAIL, JIRA_API_TOKEN, PROJECTS) before running.")
        sys.exit(1)

    # Correct Jira REST API v3 search endpoint
    jira_base_url = f"{JIRA_DOMAIN}/rest/api/3/search"
    
    # Format the PROJECTS list into a JQL-friendly string
    project_list_jql = ", ".join(f'"{p}"' for p in PROJECTS)

    # Construct the JQL query: Find issues in selected projects that are 'Done', ordered by last update
    jql_query = f'project in ({project_list_jql}) AND status = "{JQL_STATUS}" ORDER BY updated DESC'

    print(f"--- Running JQL Query: {jql_query} ---")
    print(f"API Endpoint: {jira_base_url}")
    print("Fetching data from Jira...")

    # Prepare Basic Authentication Header
    auth_string = f"{JIRA_EMAIL}:{JIRA_API_TOKEN}"
    encoded_auth = base64.b64encode(auth_string.encode('utf-8')).decode('utf-8')
    
    headers = {
        "Accept": "application/json",
        "Content-Type": "application/json",
        "Authorization": f"Basic {encoded_auth}"
    }
    
    start_at = 0
    all_issues = []
    
    # Loop to handle pagination, fetching tickets in batches
    while True:
        params = {
            "jql": jql_query,
            "fields": "summary", # Only fetch the summary (title) field
            "maxResults": MAX_RESULTS,
            "startAt": start_at
        }
        
        # Construct the full URL with query parameters
        url_with_params = f"{jira_base_url}?{urllib.parse.urlencode(params)}"

        try:
            # Create the request object
            req = urllib.request.Request(url_with_params, headers=headers)
            
            # Open the URL and get the response
            with urllib.request.urlopen(req, timeout=TIMEOUT) as response:
                # Read the response body
                response_body = response.read().decode('utf-8')
                data = json.loads(response_body)

            issues = data.get("issues", [])
            total_results = data.get("total", 0)

            all_issues.extend(issues)
            print(f"  > Fetched {len(all_issues)} of {total_results} total issues...")

            # If the number of issues collected equals the total count, we are done
            if len(all_issues) >= total_results:
                break
            
            # Update start_at for the next page
            start_at += MAX_RESULTS

            if not issues:
                break # Stop if no issues are returned (end of data)
                
        except urllib.error.HTTPError as e:
            # Handle Jira/API errors (e.g., 401 Unauthorized, 404 Not Found)
            # e.code contains the status code
            print(f"\nERROR: HTTP request failed. Status Code: {e.code}")
            print(f"URL: {url_with_params}")
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
            print("  - Projects not found or no access")
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
    
    # Print summary before checking if issues exist
    print(f"\n--- Query Summary ---")
    print(f"Total issues found: {len(all_issues)}")
    if len(all_issues) == 0:
        print(f"JQL Query used: {jql_query}")
        print(f"Projects searched: {', '.join(PROJECTS)}")
        print(f"Status filter: {JQL_STATUS}")
        print("\nPossible reasons for no tickets:")
        print("  1. No tickets in 'Done' status for the specified projects")
        print("  2. Project key(s) might be incorrect (check if 'EN' is correct)")
        print("  3. Status name might be different (e.g., 'DONE', 'Closed', etc.)")
        print("  4. Authentication or permission issues")
            
    # --- GENERATE RELEASE NOTE OUTPUT ---
    
    if not all_issues:
        print("\nSUCCESS: No 'Done' tickets found for the specified projects.")
        return

    # Get chart version
    chart_version = get_chart_version()
    
    # Extract titles and format as a bulleted list: [KEY]: [SUMMARY]
    release_note_titles = [f"- {issue['key']}: {issue['fields']['summary']}" for issue in all_issues]

    # Create the final formatted release note
    release_notes_output = f"""
=====================================================
            RELEASE NOTES - Version {chart_version}
            Date: {datetime.now().strftime('%Y-%m-%d')}
=====================================================

Total Tickets: {len(all_issues)}
Projects Queried: {', '.join(PROJECTS)}
Jira Status Filter: {JQL_STATUS}

--- Ticket Summaries ---

{chr(10).join(release_note_titles)}

=====================================================
"""

    # Write to file in charts/tensorleap directory
    script_dir = os.path.dirname(os.path.dirname(__file__))
    release_notes_file = os.path.join(script_dir, "charts", "tensorleap", "RELEASE_NOTES.md")
    
    # Check if file exists and read existing content
    existing_content = ""
    if os.path.exists(release_notes_file):
        with open(release_notes_file, 'r') as f:
            existing_content = f.read()
    
    # Write new content at the top, followed by existing content with a separator
    with open(release_notes_file, 'w') as f:
        f.write(release_notes_output)
        if existing_content:
            f.write("\n\n")
            f.write("=" * 60 + "\n")
            f.write("=" * 60 + "\n")
            f.write("\n")
            f.write(existing_content)
    
    print(f"Release notes written to: {release_notes_file}")
    print(f"Version: {chart_version}")
    print(f"Total tickets: {len(all_issues)}")


if __name__ == "__main__":
    generate_release_notes()