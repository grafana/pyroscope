local nyc = import 'dashboards/nyc.json';
local node_exporter = import 'prometheus/node-exporter-full.json';

{
  nyc: nyc,
  node_exporter: node_exporter,
}
