local alertmanager = import 'alertmanager/alertmanager.libsonnet';
local alertmanager_slack = import 'alertmanager/slack.libsonnet';

alertmanager +
alertmanager_slack +
{
  local replicas = self._config.alertmanager_cluster_self.replicas,
  local isGlobal = self._config.alertmanager_cluster_self.global,
  local isGossiping = replicas > 1 || isGlobal,
  local peers = $.buildPeers(
    if isGlobal
    then {
      [am]: $.alertmanagers[am]
      for am in std.objectFields($.alertmanagers)
      if $.alertmanagers[am].global
    }
    else {
      [$._config.cluster_name]: $.alertmanagers[$._config.cluster_name],
    }
  ),

  _config+:: {
    alertmanager_peers: peers,
    alertmanager_gossip_port: 9094,
    alertmanager_replicas: replicas,
    alertmanager_external_hostname: 'http://alertmanager.%(alertmanager_namespace)s.svc.%(cluster_dns_suffix)s' % self,
  },

  alertmanager_container+:: (
    if isGossiping
    then self.isGossiping(
      $._config.alertmanager_peers,
      $._config.alertmanager_gossip_port
    ).alertmanager_container
    else {}
  ),

  alertmanager_config_map:
    if replicas > 0
    then super.alertmanager_config_map
    else {},

  alertmanager_statefulset:
    if replicas > 0
    then super.alertmanager_statefulset
    else {},

  alertmanager_service:
    if replicas > 0
    then super.alertmanager_service
    else {},
}
