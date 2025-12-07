import requests
from requests.auth import HTTPBasicAuth
import json

url = "https://tensorleap.atlassian.net/rest/api/3/search/jql"

auth = HTTPBasicAuth("asaf.yehezkel@tensorleap.ai", "ASAFS-TOKEN")

headers = {
  "Accept": "application/json",
  "Content-Type": "application/json"
}

# POST endpoint requires JSON body, not query parameters
payload = {
  'jql': 'project = EN and status = Done',
  'maxResults': 100,
  'fields': ['summary', 'key', 'status', 'created', 'updated', 'project']
}

response = requests.request(
   "POST",
   url,
   headers=headers,
   json=payload,
   auth=auth
)

# Check response status and headers
print(f"Status Code: {response.status_code}")
print(f"Response Headers: {dict(response.headers)}")
print(f"\nResponse Text:\n{response.text}\n")

# Check for authentication failure
login_reason = response.headers.get('X-Seraph-Loginreason', '')
if login_reason == 'AUTHENTICATED_FAILED':
    print("❌ AUTHENTICATION FAILED!")
    print("Check your credentials")
else:
    # Try to parse JSON
    try:
        data = response.json()
        print(json.dumps(data, sort_keys=True, indent=4, separators=(",", ": ")))
    except json.JSONDecodeError:
        print("Error: Response is not valid JSON")
        print(f"Raw response: {response.text}")