/* eslint-disable no-await-in-loop */
const pprof = require('pprof')
const fetch = require('node-fetch');
const fs = require('fs');
const FormData = require('form-data');
const { builtinModules } = require('module');

let isContinueProfiling = true;
let serverName = 'http://localhost:4040';

async function uploadProfile(profile) {
  const buf = await pprof.encode(profile);

  // eslint-disable-next-line no-loop-func
  // fs.writeFile(`app-${chunk}.pb.gz`, buf, (err) => {
  //   if (err) throw err;
  //   console.log('Chunk written');
  //   chunk += 1;
  // });

  const formData = new FormData();
  formData.append('profile', buf, {
    knownLength: buf.byteLength,
    contentType: 'text/json',
    filename: 'profile',
  });

  // Here we assume it's make dev (or make server) running
  return fetch(
    `${serverName}/ingest?name=nodej&sampleRate=100&spyName=nodejs`,
    {
      method: 'POST',
      headers: formData.getHeaders(),
      body: formData,
    }
  )
    .then(
      (r) => r,
      (e) => console.error(e)
    )
    .then(
      (r) => r,
      (e) => console.error(e)
    );
}


async function start(options) {
  
  isContinueProfiling = true;

  while (isContinueProfiling) {
    console.log("Starting profile... ")
    const profile = await pprof.time.profile({
        name: options.name,
        lineNumbers: options.lineNumbers,
        durationMillis: 10000, // time in milliseconds for which to collect profile. 10 secods by default
    });
    await uploadProfile(profile);
  }
}


async function stop() {
  isContinueProfiling = false;
}

module.exports = {
    start,
    stop
};