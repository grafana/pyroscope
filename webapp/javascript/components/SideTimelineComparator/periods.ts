export const defaultcomparisonPeriod = {
  label: '24 hours prior',
  ms: 86400 * 1000,
};

export const comparisonPeriods = [
  [
    {
      label: '1 hour prior',
      ms: 3600 * 1000,
    },
    {
      label: '12 hours prior',
      ms: 43200 * 1000,
    },
    defaultcomparisonPeriod,
  ],
  [
    {
      label: '1 week prior',
      ms: 604800 * 1000,
    },
    {
      label: '2 weeks prior',
      ms: 1209600 * 1000,
    },
    {
      label: '30 days prior',
      ms: 2592000 * 1000,
    },
  ],
];
