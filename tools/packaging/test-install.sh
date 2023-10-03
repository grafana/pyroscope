#!/usr/bin/env bash
#
## This script helps testing packages functionally, for this docker is used including a running systemd)
#
## Usage
# Test installing using rpm repos:
# $ ./test-install.sh rpm
# Test installing using apt repos:
# $ ./test-install.sh deb
# Test installing local deb/rpm file
# $ ./test-install.sh ../../dist/pyroscope_1.1.0-rc1_linux_arm64.rpm

set -euo pipefail

IMAGE_DEBIAN=jrei/systemd-debian:12
IMAGE_REDHAT=jrei/systemd-centos:8


# Check if we should be using deb or rpm
case ${1:-} in
    deb)
        IMAGE=${IMAGE_DEBIAN}
        MODE=deb
        ;;

    *.deb)
        IMAGE=${IMAGE_DEBIAN}
        MODE=deb-file
        if [ ! -f "$1" ]; then
          echo "$1 doesn't exists."
          exit 1
        fi
        FILE=$1
        ;;

    rpm)
        IMAGE=${IMAGE_REDHAT}
        MODE=rpm
        ;;

    *.rpm)
        IMAGE=${IMAGE_REDHAT}
        MODE=rpm-file
        if [ ! -f "$1" ]; then
          echo "$1 doesn't exists."
          exit 1
        fi
        FILE=$1
        ;;

    *)
        echo "unknown first argument specify either, 'deb' to download from the apt repository, 'rpm' to download from the rpm repository or a file path to a rpm/deb file."
        exit 1
        ;;
esac


# create a debian systemd container

CONTAINER_ID=$(docker run -d --cgroupns=host --tmpfs /tmp --tmpfs /run --tmpfs /run/lock -v /sys/fs/cgroup:/sys/fs/cgroup ${IMAGE})
function container_cleanup {
  echo "Removing container ${CONTAINER_ID}"
  docker stop -t 5 "${CONTAINER_ID}"
  docker rm "${CONTAINER_ID}"
}
trap container_cleanup EXIT

case $MODE in
  deb)
    docker exec -i "$CONTAINER_ID" /bin/bash <<EOF
set -euo pipefail

apt-get update

apt-get install -y apt-transport-https software-properties-common curl

mkdir -p /etc/apt/keyrings/
curl -sL -o - https://apt.grafana.com/gpg.key | gpg --dearmor | tee /etc/apt/keyrings/grafana.gpg > /dev/null

echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" | tee -a /etc/apt/sources.list.d/grafana.list

apt-get update

apt-get install pyroscope

EOF
    ;;
  deb-file)
    docker cp "$FILE" "${CONTAINER_ID}:/root/pyroscope.deb"
    docker exec -i "$CONTAINER_ID" /bin/bash <<EOF
set -euo pipefail

apt-get update

apt-get install -y curl

dpkg -i /root/pyroscope.deb

EOF
    ;;
  rpm)
    docker exec -i "$CONTAINER_ID" /bin/bash <<EOF
set -euo pipefail

sed -i -e "s|mirrorlist=|#mirrorlist=|g" /etc/yum.repos.d/CentOS-*
sed -i -e "s|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g" /etc/yum.repos.d/CentOS-*

yum install -y curl

curl -sL -o /tmp/gpg.key https://rpm.grafana.com/gpg.key

rpm --import /tmp/gpg.key

cat > /etc/yum.repos.d/grafana.repo <<EONF
[grafana]
name=grafana
baseurl=https://rpm.grafana.com
repo_gpgcheck=1
enabled=1
gpgcheck=1
gpgkey=https://rpm.grafana.com/gpg.key
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
EONF

dnf -y install pyroscope

EOF
    ;;
  rpm-file)
    docker cp "$FILE" "${CONTAINER_ID}:/root/pyroscope.rpm"
    docker exec -i "$CONTAINER_ID" /bin/bash <<"EOF"
set -euo pipefail

sed -i -e "s|mirrorlist=|#mirrorlist=|g" /etc/yum.repos.d/CentOS-*
sed -i -e "s|#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|g" /etc/yum.repos.d/CentOS-*

yum install -y curl

rpm -i /root/pyroscope.rpm

EOF
    ;;
    *)
        echo "unknown MODE"
        exit 1
esac


echo "installed successfully"


echo "test health"
docker exec -i "$CONTAINER_ID" /bin/bash <<"EOF"

sleep 1

# show systemd unit status
systemctl status pyroscope

url=http://127.0.0.1:4040/ready
timeout -s TERM 300 bash -c \
  'while [[ "$(curl -s -o /dev/null -L -w ''%{http_code}'' '${url}')" != "200" ]];\
    do echo "Waiting for '${url}'" && sleep 2;\
    done'
EOF
