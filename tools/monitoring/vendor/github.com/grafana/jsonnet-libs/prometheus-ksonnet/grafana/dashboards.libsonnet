local grafana = import 'grafana/grafana.libsonnet';
{
  local configMap = $.core.v1.configMap,

  mixins+:: {},

  // convert to format expected by `grafana/grafana.libsonnet`:
  local grafanaConfig = {
                          grafanaDashboards: $.grafanaDashboards,
                        }
                        + grafana.addMixinDashboards($.mixins, mixinProto),
  grafanaDashboardFolders+:: grafanaConfig.grafanaDashboardFolders,

  // mixinProto (below) is applied to every dashboard managed by
  // prometheus-ksonnet. One thing it does is sets the UID of the dashboard
  // to a hash of the filename. This is neat, giving a consistent URL.
  // However, it does prevent users from presenting their own UID, which
  // would give them control over their dashboard URLs. This function allows
  // this hashing to be overridden:
  uidForDashboard(filename, dashboard):: std.md5(filename),

  // mixinProto allows us to reliably do `mixin.grafanaDashboards` without
  // having to check the field exists first. Some mixins don't declare all
  // the fields, and that's fine.
  //
  // We also use this to add a little "opinion":
  // - Dashboard UIDs are set to the md5 hash of their filename (overrideable).
  // - Timezone are set to be "default" (ie local).
  local mixinProto = {
    grafanaDashboards+:: {},
  } + {
    local grafanaDashboards = super.grafanaDashboards,

    grafanaDashboards+:: {
      [filename]:
        local dashboard = grafanaDashboards[filename];
        dashboard {
          uid: $.uidForDashboard(filename, dashboard),
          timezone: '',
        }
      for filename in std.objectFields(grafanaDashboards)
    },
  },
}
