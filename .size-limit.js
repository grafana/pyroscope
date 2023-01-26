module.exports = [
  {
    path: ['webapp/public/assets/app.js'],
    // ugly
    limit: '300000ms',
  },
  {
    path: ['webapp/public/assets/app.css'],
  },
  {
    path: ['webapp/public/assets/styles.css'],
  },
  {
    path: ['packages/pyroscope-flamegraph/dist/index.js'],
  },
  {
    path: ['packages/pyroscope-flamegraph/dist/index.node.js'],
  },
  {
    path: ['packages/pyroscope-flamegraph/dist/index.css'],
  },
];
