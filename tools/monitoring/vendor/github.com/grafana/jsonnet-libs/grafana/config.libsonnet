{
  _images+:: {
    grafana: 'grafana/grafana:8.2.5',
  },

  _config+:: {
    replicas: 1,
    rootUrl: '',
    provisioningDir: '/etc/grafana/provisioning',
    port: 80,
    containerPort: 3000,

    // Split configmaps into multiple files
    // 100000 is a good default, because a `kubectl` client-side apply cannot exceed 256kB,
    //   and the size of the request is doubled when setting the last-applied configuration
    //   https://github.com/kubernetes/kubectl/issues/712
    // For serverside applies, it can be increased, but keep in mind the 1MB limit of etcd
    //   https://github.com/kubernetes/kubernetes/issues/19781
    configmap_shard_size: 100000,

    labels+: {
      dashboards: {},
      datasources: {},
      notificationChannels: {},
    },
    grafana_ini+: {
      sections+: {
        server: {
          http_port: $._config.containerPort,
          router_logging: true,
          root_url: $._config.rootUrl,
        },
        analytics: {
          reporting_enabled: false,
        },
        users: {
          default_theme: 'light',
        },
        'log.frontend': {
          enabled: true,
        },
      },
    },
  },

  withImage(image):: {
    _images+:: {
      grafana: image,
    },
  },

  withGrafanaIniConfig(config):: {
    _config+:: {
      grafana_ini+: config,
    },
  },

  withTheme(theme):: self.withGrafanaIniConfig({
    sections+: {
      users+: {
        default_theme: theme,
      },
    },
  }),

  withAnonymous():: self.withGrafanaIniConfig({
    sections+: {
      'auth.anonymous': {
        enabled: true,
        org_role: 'Admin',
      },
    },
  }),

  withEnterpriseLicenseText(text):: self.withGrafanaIniConfig({
    sections+: {
      enterprise+: {
        license_text: text,
      },
    },
  }),

  withEnterpriseLicensePath(path):: self.withGrafanaIniConfig({
    sections+: {
      enterprise+: {
        license_path: path,
      },
    },
  }),

  withRootUrl(url):: {
    _config+:: {
      rootUrl: url,
    },
  },

  withReplicas(replicas):: {
    _config+:: {
      replicas: replicas,
    },
  },
}
