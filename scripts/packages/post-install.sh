#!/bin/bash

BIN_DIR=/usr/bin
DATA_DIR=/var/lib/pyroscope
LOG_DIR=/var/log/pyroscope
SCRIPT_DIR=/usr/lib/pyroscope/scripts

function install_init {
  cp -f $SCRIPT_DIR/init.sh /etc/init.d/pyroscope-server
  chmod +x /etc/init.d/pyroscope-server
}

function install_systemd {
  # Remove any existing symlinks
  rm -f /etc/systemd/system/pyroscope-server.service

  cp -f $SCRIPT_DIR/pyroscope-server.service /lib/systemd/system/pyroscope-server.service
}

function enable_systemd {
  systemctl enable pyroscope-server || true
  systemctl daemon-reload || true
}

function enable_update_rcd {
  update-rc.d pyroscope-server defaults
}

function enable_chkconfig {
  chkconfig --add pyroscope-server
}

id pyroscope &> /dev/null
if [[ $? -ne 0 ]]; then
  useradd --system -U -M pyroscope -s /bin/false -d $DATA_DIR
fi

test -d $LOG_DIR || mkdir -p $LOG_DIR
test -d $DATA_DIR || mkdir -p $DATA_DIR
chown -R -L pyroscope:pyroscope $LOG_DIR
chown -R -L pyroscope:pyroscope $DATA_DIR
chmod 755 $LOG_DIR
chmod 755 $DATA_DIR

# Remove legacy symlink, if it exists
if [[ -L /etc/init.d/pyroscope-server ]]; then
  rm -f /etc/init.d/pyroscope-server
fi

# Add defaults file, if it doesn't exist
if [[ ! -f /etc/default/pyroscope ]]; then
  touch /etc/default/pyroscope
fi

# Distribution-specific logic
if [[ -f /etc/redhat-release ]]; then
  # RHEL-variant logic
  which systemctl &>/dev/null
  if [[ $? -eq 0 ]]; then
    install_systemd
  else
    # Assuming sysv
    install_init
  fi
elif [[ -f /etc/debian_version ]]; then
  # Debian/Ubuntu logic
  which systemctl &>/dev/null
  if [[ $? -eq 0 ]]; then
    install_systemd
  else
    # Assuming sysv
    install_init
  fi
elif [[ -f /etc/os-release ]]; then
  source /etc/os-release
  if [[ $ID = "amzn" ]]; then
    # Amazon Linux logic
    if [[ $? -eq 0 ]]; then
      install_systemd
    else
      # Assuming sysv
      install_init
    fi
  fi
fi
