# Gemini Code Analysis Agent

This script uses Google's Gemini AI model to analyze code for potential vulnerabilities, best practice deviations, and mathematical instability.

## Features

- Analyzes code files using the Gemini 2.5 Flash Preview model
- Provides a Protocol Stability Score (1-10)
- Identifies potential flaws and security issues
- Suggests repaired code when issues are found

## Prerequisites

- Node.js 18+ (which includes built-in fetch support)
- A Google Gemini API key

## Usage

### Command Line

```bash
node scripts/gemini_agent.js <filepath> --api-key=<your_key>
```

Example:
```bash
node scripts/gemini_agent.js ./pkg/model/model.go --api-key=your_gemini_api_key
```

### GitHub Actions

The script can be run automatically via GitHub Actions:

1. **Manual Trigger**: Go to Actions → "Gemini Code Analysis" → Run workflow, and specify the file path
2. **Automatic PR Analysis**: The workflow will automatically analyze changed files in pull requests (if GEMINI_API_KEY secret is configured)

## Setting up the API Key

### For Local Development

```bash
node scripts/gemini_agent.js path/to/file.go --api-key=YOUR_API_KEY
```

### For GitHub Actions

Add `GEMINI_API_KEY` as a repository secret:

1. Go to your repository settings
2. Navigate to Secrets and variables → Actions
3. Add a new secret named `GEMINI_API_KEY`
4. Paste your Google Gemini API key

## Getting a Gemini API Key

1. Visit [Google AI Studio](https://makersuite.google.com/app/apikey)
2. Sign in with your Google account
3. Create a new API key
4. Copy the key for use with this script

## Output

The script will output:
- Protocol Stability Score (1-10)
- Summary of potential flaws
- Repaired code block (if issues are found)
- "NO REPAIR NEEDED" (if code is perfect)

## Error Handling

The script includes comprehensive error handling:
- Missing API key: Shows usage instructions
- Missing file: Shows file read error
- API failures: Shows detailed error message
- Network issues: Gracefully handles fetch failures

## Note

This is a conceptual implementation designed for protocol analysis. The script can be extended to:
- Write repaired code back to files
- Comment on GitHub PRs with analysis results
- Support batch analysis of multiple files
- Integrate with CI/CD pipelines
