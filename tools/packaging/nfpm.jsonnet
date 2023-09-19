local overrides = {
  profilecli: {
    description:
      |||
        profilecli is the command-line interface to pyroscope.
      |||,
  },

  pyroscope: {
    description: |||
      Grafana Pyroscope is an open source software project for aggregating continuous
      profiling data. Continuous profiling is an observability signal that allows you
      to understand your workload's resources (CPU, memory, etc...) usage down to the
      line number.
    |||,
    contents+: [
      {
        src: './tools/packaging/pyroscope.service',
        dst: '/etc/systemd/system/pyroscope.service',
      },
      {
        src: './cmd/pyroscope/pyroscope.yaml',
        dst: '/etc/pyroscope/config.yml',
        type: 'config|noreplace',
      },
    ],
    scripts: {
      postinstall: './tools/packaging/postinstall.sh',
    },
  },
};

local name = std.extVar('name');
local arch = std.extVar('arch');
local suffix = std.extVar('suffix');

{
  name: name,
  arch: arch,
  platform: 'linux',
  version: '${DRONE_TAG}',
  section: 'default',
  provides: [name],
  maintainer: 'Grafana Labs <support@grafana.com>',
  vendor: 'Grafana Labs Inc',
  homepage: 'https://grafana.com/pyroscope',
  license: 'AGPL-3.0',
  contents: [{
    src: './dist/%s_linux_%s%s/%s' % [name, arch, suffix, name],
    dst: '/usr/bin/%s' % name,
  }],
} + overrides[name]
