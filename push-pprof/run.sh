set -ex

go build .

rsync -r -z  -v  ./ ubuntu@ec2-3-27-199-212.ap-southeast-2.compute.amazonaws.com:./v2-multi-runners
