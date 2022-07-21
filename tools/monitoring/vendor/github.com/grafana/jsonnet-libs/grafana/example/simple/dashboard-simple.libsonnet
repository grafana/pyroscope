{
  annotations: {
    list: [
      {
        builtIn: 1,
        datasource: '-- Grafana --',
        enable: true,
        hide: true,
        iconColor: 'rgba(0, 211, 255, 1)',
        name: 'Annotations & Alerts',
        type: 'dashboard',
      },
    ],
  },
  editable: true,
  gnetId: null,
  graphTooltip: 0,
  links: [],
  panels: [
    {
      datasource: null,
      fieldConfig: {
        defaults: {
          color: {
            mode: 'thresholds',
          },
          custom: {},
          mappings: [],
          noValue: 'Simple',
          thresholds: {
            mode: 'absolute',
            steps: [
              {
                color: 'green',
                value: null,
              },
              {
                color: 'red',
                value: 80,
              },
            ],
          },
        },
        overrides: [],
      },
      gridPos: {
        h: 8,
        w: 12,
        x: 0,
        y: 0,
      },
      id: 2,
      options: {
        colorMode: 'value',
        graphMode: 'area',
        justifyMode: 'auto',
        orientation: 'auto',
        reduceOptions: {
          calcs: [
            'lastNotNull',
          ],
          fields: '',
          values: false,
        },
        text: {},
        textMode: 'auto',
      },
      pluginVersion: '7.4.0',
      timeFrom: null,
      timeShift: null,
      title: 'Simple Panel',
      type: 'stat',
    },
  ],
  schemaVersion: 27,
  style: 'dark',
  tags: [
    'templated',
  ],
  templating: {
    list: [],
  },
  time: {
    from: 'now-6h',
    to: 'now',
  },
  timepicker: {},
  timezone: 'browser',
  title: 'Production Overview',
  uid: 'prod-overview',
}
