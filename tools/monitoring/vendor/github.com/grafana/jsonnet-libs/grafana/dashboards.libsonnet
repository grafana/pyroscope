{
  folderID(folder)::
    local lower = std.asciiLower(folder);
    local underscore = std.strReplace(lower, '_', '-');
    local space = std.strReplace(underscore, ' ', '-');
    space,

  local folderID(folder) = self.folderID(folder),

  // add a new empty folder
  // It's super common for the dashboards in a single folder to be too
  // large to fit inside a single Kubernetes ConfigMap. In that case,
  // dashboards will be sharded into multiple ConfigMaps
  addFolder(name):: {
    grafanaDashboardFolders+:: {
      [name]: {
        id: folderID(name),
        name: name,
        dashboards: {},
      },
    },
  },

  // add a new dashboard, creating the folder if necessary
  addDashboard(name, dashboard, folder=''):: {
    grafanaDashboardFolders+:: {
      [folder]+: {
        id: folderID(folder),
        name: folder,
        dashboards+: {
          [name]+: dashboard,
        },
      },
    },
  },

  addMixinDashboards(mixins, mixinProto={}):: {
    local grafanaDashboards = super.grafanaDashboards,
    grafanaDashboardFolders+:: std.foldr(
      function(name, acc)
        acc
        + (
          if std.objectHasAll(mixins[name], 'grafanaDashboards')
             && std.length(mixins[name].grafanaDashboards) > 0
          then
            local key = (
              if std.objectHasAll(mixins[name], 'grafanaDashboardFolder')
              then $.folderID(mixins[name].grafanaDashboardFolder)
              else 'general'
            );
            {
              [key]+: {
                dashboards+: (mixins[name] + mixinProto).grafanaDashboards,
                name:
                  if std.objectHasAll(mixins[name], 'grafanaDashboardFolder')
                  then mixins[name].grafanaDashboardFolder
                  else '',
                id: $.folderID(self.name),
              },
            }
          else {}
        ),
      std.objectFields(mixins),
      {}
    ) + {
      general+: {
        dashboards+: grafanaDashboards + mixinProto,
        name: '',
        id: '',
      },
    },
  },

  grafanaDashboardFolders+:: {},
}
