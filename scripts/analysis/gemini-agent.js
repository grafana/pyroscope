// --- Gemini Agent Script (Node.js) ---

const fs = require('fs');
const path = require('path');

// NOTE: In a real Node.js environment, you would use an external library like
// 'node-fetch' for making HTTP requests, and potentially 'dotenv' for keys.
// For this example, we assume basic fetch is available or polyfilled.

const GEMINI_MODEL = 'gemini-2.5-flash-preview-09-2025';

// Use a simple prompt designed for code repair/analysis
const AGENT_SYSTEM_PROMPT = `Act as an elite protocol engineer specializing in ZK-Circuits and DAO stability. Analyze the provided code block for vulnerabilities, deviations from best practices, and mathematical instability. 
1. Assign a **Protocol Stability Score (1-10)**.
2. Provide a brief summary of **Potential Flaws**.
3. If flaws exist, provide the **REPAIRED CODE BLOCK** only. Do not provide any conversational text before the repaired code. If the code is perfect, output 'NO REPAIR NEEDED'.`;

/**
 * Executes the Gemini API call to analyze the given code snippet.
 * @param {string} codeContent - The content of the file to analyze.
 * @param {string} filePath - The name of the file being analyzed.
 * @param {string} apiKey - The Gemini API key.
 */
async function runAnalysis(codeContent, filePath, apiKey) {
    if (!apiKey) {
        console.error("Error: GEMINI_API_KEY is missing. Check GitHub Secrets configuration.");
        return;
    }

    const apiUrl = `https://generativelanguage.googleapis.com/v1beta/models/${GEMINI_MODEL}:generateContent?key=${apiKey}`;
    
    // The user query includes the file content to be analyzed
    const userQuery = `Analyze the protocol code for ${filePath}:\n\n\`\`\`\n${codeContent}\n\`\`\``;

    const payload = {
        contents: [{ parts: [{ text: userQuery }] }],
        systemInstruction: { parts: [{ text: AGENT_SYSTEM_PROMPT }] },
        // For code analysis, grounding is usually not necessary unless you need real-time data
        // tools: [{ "google_search": {} }], 
    };

    let response;
    try {
        // Simple fetch example (replace with node-fetch in a real project)
        response = await fetch(apiUrl, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const result = await response.json();
        const analysisText = result.candidates?.[0]?.content?.parts?.[0]?.text || "Agent failed to generate response.";
        
        console.log(`\n### Protocol Analysis for ${filePath} ###`);
        console.log(analysisText);

        // --- DEEP THINK REPAIR LOGIC ---
        // A real agent would look for the 'REPAIRED CODE BLOCK' and attempt to 
        // write it back to the file system or comment on the PR using the GitHub Token.
        const analysisComplete = analysisText.includes('REPAIRED CODE BLOCK') || analysisText.includes('NO REPAIR NEEDED');
        if (analysisComplete) {
             console.log("Analysis Complete. Check output for repair instructions.");
        }

    } catch (error) {
        console.error(`\n--- FAILED ANALYSIS for ${filePath} ---`);
        console.error(`Gemini Agent Error: ${error.message}`);
    }
}

/**
 * Main execution function to handle command-line arguments.
 */
async function main() {
    const filePath = process.argv[2]; 
    const apiKeyArg = process.argv.find(arg => arg.startsWith('--api-key='));
    
    if (!filePath || !apiKeyArg) {
        console.error("Usage: node gemini-agent.js <filepath> --api-key=<your_key>");
        return;
    }

    const apiKey = apiKeyArg.split('=')[1];

    try {
        const codeContent = fs.readFileSync(path.resolve(filePath), 'utf8');
        await runAnalysis(codeContent, filePath, apiKey);
    } catch (error) {
        console.error(`Error reading file ${filePath}: ${error.message}`);
    }
}

// Ensure fetch is available in Node.js environment
// Node.js 18+ has built-in fetch, older versions need node-fetch
if (typeof fetch === 'undefined') {
    try {
        global.fetch = require('node-fetch');
    } catch (error) {
        console.error('Error: fetch is not available. Please upgrade to Node.js 18+ or install node-fetch: npm install node-fetch');
        process.exit(1);
    }
}

main();
