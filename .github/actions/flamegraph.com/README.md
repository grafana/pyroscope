
# Running locally

You can use [`act`](https://github.com/nektos/act)

`yarn build`
`DOCKER_HOST="unix://$HOME/.colima/docker.sock" act --container-architecture linux/amd64  --workflows .github/workflows/upload-test.yml`
