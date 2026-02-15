# Gemini Agent - Code Analysis Tool

## Overview

The Gemini Agent is a Node.js-based code analysis tool that uses Google's Gemini AI to analyze code for vulnerabilities, deviations from best practices, and mathematical instability. It's particularly designed for protocol engineering, ZK-Circuits, and DAO stability analysis.

## Features

- **Protocol Stability Score**: Assigns a 1-10 score for code stability
- **Vulnerability Detection**: Identifies potential security flaws
- **Code Repair Suggestions**: Provides repaired code blocks when issues are found
- **Best Practices Analysis**: Checks for deviations from industry standards

## Prerequisites

1. Node.js installed (version 18 or higher for native fetch support)
2. Google Gemini API Key (obtain from [Google AI Studio](https://aistudio.google.com/))
3. Dependencies installed (run `yarn install` or `npm install`)

## Installation

```bash
# Install dependencies
yarn install

# Or using npm
npm install
```

## Usage

### Command Line

```bash
node scripts/analysis/gemini-agent.js <filepath> --api-key=<your_api_key>
```

### Using npm/yarn script

```bash
yarn analyze:gemini <filepath> --api-key=<your_api_key>

# Or
npm run analyze:gemini <filepath> --api-key=<your_api_key>
```

### Example

```bash
# Analyze a JavaScript file
node scripts/analysis/gemini-agent.js ./src/example.js --api-key=YOUR_GEMINI_API_KEY

# Analyze a Go file
node scripts/analysis/gemini-agent.js ./pkg/example.go --api-key=YOUR_GEMINI_API_KEY
```

## Environment Variables

For security, it's recommended to use environment variables instead of passing the API key directly:

```bash
export GEMINI_API_KEY=your_api_key_here
node scripts/analysis/gemini-agent.js <filepath> --api-key=$GEMINI_API_KEY
```

## GitHub Actions Integration

To integrate with GitHub Actions, add the following to your workflow:

```yaml
- name: Analyze Code with Gemini Agent
  env:
    GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
  run: |
    node scripts/analysis/gemini-agent.js ${{ matrix.file }} --api-key=$GEMINI_API_KEY
```

Make sure to add `GEMINI_API_KEY` to your GitHub repository secrets.

## Output

The tool provides:

1. **Protocol Stability Score**: A numerical rating (1-10)
2. **Potential Flaws Summary**: Brief description of issues found
3. **Repair Status**: Either:
   - `REPAIRED CODE BLOCK`: Contains the fixed code
   - `NO REPAIR NEEDED`: Code is in good condition

## Error Handling

The script handles common errors:
- Missing API key
- Invalid file paths
- API connection issues
- Malformed API responses

## Security Notes

⚠️ **Important Security Considerations:**

1. Never commit API keys to version control
2. Use environment variables or GitHub Secrets for API keys
3. Add API keys to `.gitignore` if storing them in config files
4. Rotate API keys regularly
5. Use least-privilege API key permissions

## Limitations

- Requires active internet connection
- Depends on Gemini API availability
- API rate limits may apply (check Google's documentation)
- Analysis quality depends on code context and complexity

## Troubleshooting

### "GEMINI_API_KEY is missing"
- Ensure you're passing the `--api-key` parameter
- Check that the API key is correctly formatted

### "HTTP error! status: 403"
- Verify your API key is valid
- Check API key permissions
- Ensure billing is enabled in Google Cloud Console

### "Error reading file"
- Verify the file path is correct
- Ensure the file exists and is readable
- Use absolute paths if relative paths fail

## Contributing

To improve the Gemini Agent:

1. Follow the existing code style
2. Test changes with multiple file types
3. Update documentation for new features
4. Consider adding unit tests

## License

This tool is part of the Grafana Pyroscope project and follows the same AGPL-3.0-only license.

## Support

For issues or questions:
- Open an issue in the repository
- Check existing documentation
- Review Google Gemini API documentation
