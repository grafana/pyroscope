/* eslint-disable no-await-in-loop */
import * as pprof from '@datadog/pprof';
import fetch from 'node-fetch';
import fs from 'fs';
import FormData from 'form-data';

async function runProfile() {
  let chunk = 0;
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const profile = await pprof.time.profile({
      durationMillis: 10000, // time in milliseconds for which to collect profile. 10 secods by default
    });

    const buf = await pprof.encode(profile);

    // eslint-disable-next-line no-loop-func
    fs.writeFile(`app-${chunk}.pb.gz`, buf, (err) => {
      if (err) throw err;
      console.log('Chunk written');
      chunk += 1;
    });

    // fs.writeFile(`app-${chunk}.json`, JSON.stringify(profile), (err) => {
    //   if (err) throw err;
    //   console.log('Chunk-json written');
    // });

    const formData = new FormData();
    formData.append('profile', buf, {
      knownLength: buf.byteLength,
      contentType: 'text/json',
      filename: 'profile',
    });

    // Here we assume it's make dev (or make server) running
    fetch(
      'http://localhost:4040/ingest?name=nodej&sampleRate=100&spyName=nodejs',
      {
        method: 'POST',
        headers: formData.getHeaders(),
        body: formData,
      }
    )
      .then(
        (r) => console.log(r),
        (e) => console.error(e)
      )
      .then(
        (r) => console.log(r),
        (e) => console.error(e)
      );
  }
}

runProfile();
