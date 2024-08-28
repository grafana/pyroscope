import requests
from requests.auth import HTTPBasicAuth

# NOTE: For original docs visit information visit:
# https://grafana.com/docs/pyroscope/next/configure-server/about-server-api/

# Authentication details if using cloud
basic_auth_username = '<username>'  # Replace with your Grafana Cloud stack user
basic_auth_password = '<password>'  # Replace with your Grafana Cloud API key
pyroscope_server = 'https://profiles-prod-001.grafana.net'

# If not using cloud, use the following:
# pyroscope_server = 'http://localhost:4040' # replace with your server address:port

application_name = 'my_application_name'
query = f'process_cpu:cpu:nanoseconds:cpu:nanoseconds{{service_name="{application_name}"}}'
query_from = 'now-1h'
pyroscope_url = f'{pyroscope_server}/pyroscope/render?query={query}&from={query_from}'

# Sending the request with authentication
response = requests.get(
    pyroscope_url,
    auth=HTTPBasicAuth(basic_auth_username, basic_auth_password)
)

# Checking the response
if response.status_code == 200:
    print(response.text)
else:
    print(f"Failed to query data. Status code: {response.status_code}, Message: {response.text}")
