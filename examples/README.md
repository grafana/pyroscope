# Examples

These are example projects we set up with pyroscope for you to try out. You'll need `docker` + `docker-compose` to run them:

```shell
cd golang
docker-compose up --build
```

These are very simple projects where the application is basically one `while true` loop and inside that loop it calls a slow function and a fast function. Slow function takes about 80% of the time and the fast one takes about 20%.
