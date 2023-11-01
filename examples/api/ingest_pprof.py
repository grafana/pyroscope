import requests
import json
from requests.auth import HTTPBasicAuth

# NOTE: For original docs visit information visit: 
# https://grafana.com/docs/pyroscope/next/configure-server/about-server-api/ 

# Specify the path to your pprof file, Pyroscope server URL, and application name
pprof_file_path = 'path/to/your/pprof-file.pprof'

# Authentication details if using cloud
basic_auth_username = "<username>"  # Replace with your Grafana Cloud stack user
basic_auth_password = "<password>"  # Replace with your Grafana Cloud API key
pyroscope_server = 'https://profiles-prod-001.grafana.net'

# If not using cloud, use the following:
# pyroscope_server = 'http://localhost:4040' # replace with your server address:port

# Specify the application name
application_name = 'my_application_name'  # Replace with your application's name

# Construct the Pyroscope ingest URL
pyroscope_url = f'{pyroscope_server}/ingest?name={application_name}'

# # Define your sample type configuration (Modify as needed)
# sample_type_config = {
#     "your_sample_type": {  # Replace 'your_sample_type' with the actual sample type
#         "units": "your_units",  # e.g., "samples", "bytes"
#         "aggregation": "your_aggregation",  # e.g., "sum", "average"
#         "display-name": "your_display_name",
#         "sampled": True or False  # Set to True or False based on your data
#     }
# }

# Sample type configuration
sample_type_config = {
  "cpu": {
    "units": "samples",
    "aggregation": "sum",
    "display-name": "cpu_samples",
    "sampled": True
  }
}

# Form data to be sent
multipart_form_data = {
    'profile': ('example.pprof', open(pprof_file_path, 'rb')),
    'sample_type_config': ('config.json', json.dumps(sample_type_config))
}

# Sending the request with authentication
response = requests.post(
    pyroscope_url,
    files=multipart_form_data,
    auth=HTTPBasicAuth(basic_auth_username, basic_auth_password)
)

# Checking the response
if response.status_code == 200:
    print("Profile data successfully sent to Pyroscope.")
else:
    print(f"Failed to send data. Status code: {response.status_code}, Message: {response.text}")
