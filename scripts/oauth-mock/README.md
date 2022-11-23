### What is this?

This is a mock OAuth server for testing purposes. It allows you to run pyroscope with oauth enabled locally by mimicking gitlab oauth server.


### Usage

To run the mock server, run the following command:
```shell
node ./scripts/oauth-mock/oauth-mock.js
```

To run pyroscope with the right config run:
```shell
make build
bin/pyroscope server -config scripts/oauth-mock/pyroscope-config.yml
```

Then go to http://localhost:4040/ and pyroscope should send you to log in via GitLab (which is really just our mock in this case).

