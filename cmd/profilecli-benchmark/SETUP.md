# Google Sheets Setup Guide

This guide will help you set up Google Sheets API access for the profilecli-benchmark tool.

## Prerequisites

- A Google account
- Access to Google Cloud Console

## Step 1: Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Click "Select a project" dropdown at the top
3. Click "NEW PROJECT"
4. Enter a project name (e.g., "profilecli-benchmark")
5. Click "CREATE"

## Step 2: Enable Google Sheets API

1. In the Google Cloud Console, ensure your new project is selected
2. Go to "APIs & Services" > "Library" (or visit https://console.cloud.google.com/apis/library)
3. Search for "Google Sheets API"
4. Click on "Google Sheets API"
5. Click "ENABLE"

## Step 3: Create a Service Account

1. Go to "APIs & Services" > "Credentials" (or visit https://console.cloud.google.com/apis/credentials)
2. Click "CREATE CREDENTIALS" at the top
3. Select "Service account"
4. Fill in the details:
   - Service account name: `profilecli-benchmark`
   - Service account ID: (auto-generated, but you can customize)
   - Description: "Service account for profilecli benchmark tool"
5. Click "CREATE AND CONTINUE"
6. Skip the optional steps and click "DONE"

## Step 4: Create and Download Credentials

1. On the Credentials page, find your newly created service account under "Service Accounts"
2. Click on the service account email (e.g., `profilecli-benchmark@project-id.iam.gserviceaccount.com`)
3. Go to the "KEYS" tab
4. Click "ADD KEY" > "Create new key"
5. Select "JSON" format
6. Click "CREATE"
7. The credentials file will be downloaded automatically
8. Save this file securely (e.g., `~/profilecli-benchmark-credentials.json`)

**IMPORTANT**: Keep this credentials file secure. It provides access to your Google Cloud resources.

## Step 5: Create a Google Spreadsheet

1. Go to [Google Sheets](https://sheets.google.com/)
2. Click the "+" button or "Blank" to create a new spreadsheet
3. Give it a name (e.g., "ProfileCLI Benchmark Results")
4. Copy the spreadsheet ID from the URL:
   ```
   https://docs.google.com/spreadsheets/d/SPREADSHEET_ID_HERE/edit
   ```

## Step 6: Share the Spreadsheet with the Service Account

1. In your Google Spreadsheet, click the "Share" button
2. Paste the service account email (from Step 3) into the "Add people and groups" field
   - Example: `profilecli-benchmark@project-id.iam.gserviceaccount.com`
3. Give it "Editor" permissions
4. Uncheck "Notify people" (no need to send an email to a service account)
5. Click "Share"

## Step 7: Test the Setup

Run the benchmark tool in dry-run mode to verify everything works:

```bash
cd cmd/profilecli-benchmark
GOWORK=off go build -o profilecli-benchmark .

# Test without Google Sheets (dry-run)
./profilecli-benchmark --dry-run --profilecli ../../profilecli

# Test with Google Sheets
./profilecli-benchmark \
  --profilecli ../../profilecli \
  --spreadsheet-id "YOUR_SPREADSHEET_ID" \
  --credentials "/path/to/credentials.json"
```

## Environment Variables (Optional)

You can also set environment variables to avoid passing flags every time:

```bash
export PROFILECLI_SPREADSHEET_ID="your_spreadsheet_id"
export PROFILECLI_CREDENTIALS="$HOME/profilecli-benchmark-credentials.json"

./profilecli-benchmark --profilecli ../../profilecli
```

## Troubleshooting

### "Permission denied" error

- Make sure you shared the spreadsheet with the service account email
- Verify the service account has "Editor" permissions

### "API not enabled" error

- Go back to Step 2 and ensure Google Sheets API is enabled
- Wait a few minutes for the API to be fully enabled

### "Invalid credentials" error

- Verify the credentials file path is correct
- Ensure the JSON file is valid and not corrupted
- Make sure you're using the correct service account credentials

### "Spreadsheet not found" error

- Double-check the spreadsheet ID
- Ensure the spreadsheet is shared with the service account
- Verify the service account has access to the spreadsheet

## Security Best Practices

1. **Never commit credentials to git**: Add `*.json` to `.gitignore`
2. **Restrict permissions**: Only give the service account access to the specific spreadsheet
3. **Rotate keys periodically**: Create new keys and delete old ones every few months
4. **Use different projects**: Use separate Google Cloud projects for different environments (dev, prod)
5. **Monitor usage**: Check the Google Cloud Console for unusual API usage

## Cost Considerations

- Google Sheets API has a free quota:
  - 100 requests per 100 seconds per user
  - 500 requests per 100 seconds per project
- This benchmark tool makes a small number of requests, so it should stay within the free tier
- Monitor your usage in the [Google Cloud Console](https://console.cloud.google.com/apis/dashboard)

## Example Complete Workflow

```bash
# 1. Build profilecli
cd /Users/christian/git/github.com/grafana/pyroscope
make go/bin

# 2. Build benchmark tool
cd cmd/profilecli-benchmark
GOWORK=off go build

# 3. Set up environment
export SPREADSHEET_ID="1abc...xyz"
export CREDENTIALS="$HOME/profilecli-benchmark-credentials.json"

# 4. Run benchmark
./profilecli-benchmark \
  --profilecli ../../profilecli \
  --spreadsheet-id "$SPREADSHEET_ID" \
  --credentials "$CREDENTIALS"
```

## Additional Resources

- [Google Sheets API Documentation](https://developers.google.com/sheets/api)
- [Service Account Documentation](https://cloud.google.com/iam/docs/service-accounts)
- [Google Cloud Console](https://console.cloud.google.com/)
