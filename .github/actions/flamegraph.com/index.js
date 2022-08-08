const core = require('@actions/core');
const httpm = require('@actions/http-client');
const fs = require('fs').promises;
const { SUMMARY_ENV_VAR } = require('@actions/core/lib/summary');

const http = new httpm.HttpClient('');

const isSummaryEnabled = () => {};

async function upload() {
  const filepath = core.getInput('file');

  const file = await fs.readFile(filepath, { encoding: 'base64' });

  //const baseUrl = 'http://192.168.0.106:3000';
  const baseUrl = 'https://www.flamegraph.com';
  //  const baseUrl = 'https://eo8m4nymx1a0ouk.m.pipedream.net';

  const data = {
    filename: filepath,
    name: filepath,
    profile: file,
  };

  const res = await http.postJson(`${baseUrl}/api/upload/v1`, {
    ...data,
  });

  core.setOutput('url', res.result.url);

  // TODO: summary is disabled in act?
  if (process.env[SUMMARY_ENV_VAR] !== undefined) {
    await core.summary
      .addHeading('Results', 2)
      .addLink('View Run in Flamegraph.com', res.result.url)
      .addImage(
        `https://flamegraph.com/api/preview/${res.result.key}`,
        'Flamegraph'
      )
      .write();
  }

  //  https://flamegraph.com/api/preview/17c74be5-175c-11ed-94d9-f6c0eb336362
}

upload().catch((error) => core.setFailed(error.message));
