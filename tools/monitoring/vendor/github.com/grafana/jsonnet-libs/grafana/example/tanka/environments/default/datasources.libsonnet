local grafana = import 'grafana/grafana.libsonnet';

{
  prometheus:
    grafana.datasource.new(
      'Prometheus',
      'http://prometheus-server.prometheus',
      'prometheus',
      true,
    ) +
    grafana.datasource.withHttpMethod('POST'),
  sun_and_moon:
    grafana.datasource.new(
      'NYC',
      null,
      'fetzerch-sunandmoon-datasource',
    ) +
    grafana.datasource.withJsonData({
      latitude: 40.7128,
      longitude: -74.0060,
    }),
}
