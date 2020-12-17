#!/bin/bash

function disable_systemd {
  systemctl disable pyroscope-server
  rm -f /lib/systemd/system/pyroscope-server.service
}

function disable_update_rcd {
  update-rc.d -f pyroscope-server remove
  rm -f /etc/init.d/pyroscope-server
}

function disable_chkconfig {
  chkconfig --del pyroscope-server
  rm -f /etc/init.d/pyroscope-server
}

if [[ -f /etc/redhat-release ]]; then
  # RHEL-variant logic
  if [[ "$1" = "0" ]]; then
    # pyroscope is no longer installed, remove from init system
    rm -f /etc/default/pyroscope

    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
        disable_systemd
    else
        # Assuming sysv
        disable_chkconfig
    fi
  fi
elif [[ -f /etc/lsb-release ]]; then
  # Debian/Ubuntu logic
  if [[ "$1" != "upgrade" ]]; then
    # Remove/purge
    rm -f /etc/default/pyroscope

    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
        disable_systemd
    else
        # Assuming sysv
        disable_update_rcd
    fi
  fi
elif [[ -f /etc/os-release ]]; then
  source /etc/os-release
  if [[ $ID = "amzn" ]]; then
    # Amazon Linux logic
    if [[ "$1" = "0" ]]; then
        # pyroscope is no longer installed, remove from init system
        rm -f /etc/default/pyroscope
        which systemctl &>/dev/null
        if [[ $? -eq 0 ]]; then
            disable_systemd
        else
            # Assuming sysv
            disable_chkconfig
        fi
    fi
  fi
fi
