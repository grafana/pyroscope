local k = import 'ksonnet-util/kausal.libsonnet';
local configMap = k.core.v1.configMap;
local deployment = k.apps.v1.deployment;

{
  // grafana_ini configmap
  grafana_ini_config_map:
    configMap.new('grafana-config') +
    configMap.withData({ 'grafana.ini': std.manifestIni($._config.grafana_ini) }),

  // datasource provisioning configmap
  grafana_datasource_config_map:
    configMap.new('grafana-datasources') +
    configMap.withDataMixin({
      ['%s.yml' % name]: k.util.manifestYaml({
        apiVersion: 1,
        datasources: [$.grafanaDatasources[name]],
      })
      for name in std.objectFields($.grafanaDatasources)
    })
    + configMap.mixin.metadata.withLabels($._config.labels.datasources),

  // notification channel provisioning configmap
  notification_channel_config_map:
    configMap.new('grafana-notification-channels') +
    configMap.withDataMixin({
      [name]: k.util.manifestYaml({
        notifiers: [
          $.grafanaNotificationChannels[name],
        ],
      })
      for name in std.objectFields($.grafanaNotificationChannels)
    }) +
    configMap.mixin.metadata.withLabels($._config.labels.notificationChannels),

  local prefix(name) = if name == '' then 'dashboards' else 'dashboards-%s' % name,

  // dashboard provisioning configmaps
  dashboard_provisioning_config_map:
    configMap.new('grafana-dashboard-provisioning') +
    configMap.withData({
      'dashboards.yml': k.util.manifestYaml({
        apiVersion: 1,
        providers: [
          {
            name: prefix($.grafanaDashboardFolders[name].id),
            orgId: 1,
            folder: $.grafanaDashboardFolders[name].name,
            type: 'file',
            disableDeletion: true,
            editable: false,
            options: {
              path: '/grafana/%s' % prefix($.grafanaDashboardFolders[name].id),
            },
          }
          for name in std.objectFields($.grafanaDashboardFolders)
        ],
      }),
    }),

  // Dashboard JSON configmaps
  // An effort is made to balance config maps by
  //   matching the smallest dashboards with the biggest ones
  local calculateShards(folder) =
    // Sort dashboards descending by size
    local dashboards = std.sort([
      {
        name: if std.endsWith(name, '.json') then name else '%s.json' % name,
        content: std.toString(folder.dashboards[name]),
      }
      for name in std.objectFields(folder.dashboards)
    ], function(d) -std.length(d.content));
    local count = std.length(dashboards);

    // Shard configmaps at around 100kB per shard
    local totalCharacters = std.foldl(function(x, y) x + y, [std.length(d.content) for d in dashboards], 0);
    local shardCount = std.min(count, std.ceil(totalCharacters / $._config.configmap_shard_size));
    {
      // Calculate the number of dashboards per shard
      // This is skewed towards tail dashboards (smallest ones)
      // For example, if we need 3 per shard, it will be 1 big and 2 smalls
      local perShard = std.floor(count / shardCount),
      local perShardHead = std.floor(perShard / 2),
      local perShardTail = std.ceil(perShard / 2),

      // perShard is a floor, so we can have a remainder
      // It is taken from the end of the array (smallest dashboards)
      // At the end of the loop, we add the remainder to the last shard
      local maxTail = shardCount * perShard,
      local remainder = count - maxTail,
      ['%s-%d' % [prefix(folder.id), shard]]+:
        local head = shard * perShardHead;
        local nextHead = head + perShardHead;
        local tail = maxTail - (shard * perShardTail);
        local nextTail = tail - perShardTail;

        {
          [dashboard.name]: dashboard.content
          for dashboard in
            // Dashboards from beginning + from end + remainder for last shard
            std.slice(dashboards, head, nextHead, 1)
            + std.slice(dashboards, nextTail, tail, 1)
            + if shard == shardCount - 1 && remainder > 0 then std.slice(dashboards, maxTail, maxTail + remainder, 1) else []
        }
      for shard in std.range(0, shardCount - 1)
      if count > 0
    },


  local shardedConfigMaps(folder) =
    local shards = calculateShards(folder);
    {
      [shardName]+:
        configMap.new(shardName) +
        configMap.withDataMixin(shards[shardName])
        + configMap.mixin.metadata.withLabels($._config.labels.dashboards)
      for shardName in std.objectFields(shards)
    },

  dashboard_folders_config_maps: std.foldl(
    function(acc, name)
      acc + shardedConfigMaps($.grafanaDashboardFolders[name]),
    std.objectFields($.grafanaDashboardFolders),
    {},
  ),

  // Helper to mount a variable number of sharded config maps.
  local shardedMounts(folder) =
    local shards = calculateShards(folder);
    [
      k.util.volumeMountItem(shard, '/grafana/%s/%s' % [prefix(folder.id), shard])
      for shard in std.objectFields(shards)
    ],

  // configmap mounts for use within statefulset/deployment
  configmap_mounts::
    local mounts =
      [
        k.util.configMapVolumeMountItem($.grafana_ini_config_map, '/etc/grafana-config'),
        k.util.configMapVolumeMountItem($.dashboard_provisioning_config_map, '%(provisioningDir)s/dashboards' % $._config),
        k.util.configMapVolumeMountItem($.grafana_datasource_config_map, '%(provisioningDir)s/datasources' % $._config),
        k.util.configMapVolumeMountItem($.notification_channel_config_map, '%(provisioningDir)s/notifiers' % $._config),
      ]
      + std.flattenArrays([
        shardedMounts($.grafanaDashboardFolders[folder])
        for folder in std.objectFields($.grafanaDashboardFolders)
      ]);

    k.util.volumeMounts(mounts)
    + deployment.mixin.spec.template.metadata.withAnnotationsMixin({
      'grafana-dashboards-hash': std.md5(std.toString($.grafanaDashboardFolders)),
    }),
}
