/* eslint-disable */
const pprof = require('pprof');
const fetch = require('node-fetch');
const fs = require('fs');
const FormData = require('form-data');
const { builtinModules } = require('module');

const INTERVAL = 10000;

let isContinueProfiling = true;
const serverName = 'http://localhost:4040';
let chunk = 0;

async function uploadProfile(profile, tags) {
  // TODO: Tag profile here

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
    `${serverName}/ingest?name=nodejs&sampleRate=100&spyName=nodejs`,
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
  const sm = await pprof.SourceMapper.create([process.cwd()]);
  console.log(sm);
  while (isContinueProfiling) {
    console.log('Starting profile... ');
    const profile = await pprof.time.profile({
      name: options.name,
      lineNumbers: true,
      sourceMapper: sm,
      durationMillis: INTERVAL, // time in milliseconds for which to collect profile. 10 secods by default
    });
    await uploadProfile(profile, options.tags);
  }
}

// Could be false or a function to stop heap profiling
let isHeapProfilingStarted = false;

async function startHeapProfiling({ tags }) {
  const intervalBytes = 1024 * 512;
  const stackDepth = 32;

  if (isHeapProfilingStarted) return false;

  const sm = await pprof.SourceMapper.create([process.cwd()]);

  pprof.heap.start(intervalBytes, stackDepth);

  isHeapProfileStarted = setInterval(async () => {
    console.log('Collecting heap profile');
    const profile = pprof.heap.profile(undefined, sm);
    console.log('Heap profile collected...');
    await uploadProfile(profile, options.tags);
    console.log('Heap profile uploaded...');
  }, INTERVAL);
}

function stopHeapProfiling() {
  if (isHeapProfilingStarted) {
    console.log('Stopping heap profiling');
    isHeapProfilingStarted();
    isHeapProfilingStarted = false;
  }
}

async function stop() {
  isContinueProfiling = false;
}

module.exports = {
  start,
  stop,
  startHeapProfiling,
  stopHeapProfiling,
};
