module.exports = [
  {
    webpack: false,
    path: ['webapp/public/assets/*.js', 'webapp/public/assets/*.css'],
    // ugly
    limit: '15000ms',
  },
];
