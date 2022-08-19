import React, { useMemo } from 'react';
import type { ApexOptions } from 'apexcharts';
import Chart from 'react-apexcharts';
import globalTemperatureJSON from './global-temperature.json';

const options = {
  dataLabels: {
    enabled: false,
  },
  colors: ['#800080'],
  title: {
    text: '<Heat map title>',
  },
  xaxis: {
    tickAmount: 6,
  },
};

// to convert mock tempreture to apex series format
// remove after fetch implementation
const getApexSeriesFromMockJson = (): ApexOptions['series'] => {
  return globalTemperatureJSON.monthlyVariance.reduce(
    (acc, { year, month, variance }) => {
      acc[month - 1].name = month;
      acc[month - 1].data.push({ x: year.toString(), y: variance > 0 ? 1 : 0 });

      return acc;
    },
    new Array(12).fill({ data: [], name: '' })
  );
};

// getApexSeriesFromMockJson();

// {
//   "x": 1753,
//   "y": 1,
//   "variance": -1.366
// },
// {
//   "year": 1753,
//   "month": 2,
//   "variance": -2.223
// },
// {
//   "year": 1753,
//   "month": 3,
//   "variance": 0.211
// },

// {
//   "year": 1754,
//   "month": 1,
//   "variance": -1.748
// },
// {
//   "year": 1754,
//   "month": 2,
//   "variance": -4.175
// },
// {
//   "year": 1754,
//   "month": 3,
//   "variance": -1.448
// },

const series = [
  {
    name: 'Series 1',
    data: [
      {
        x: '2000',
        y: 0,
      },
      {
        x: '2003',
        y: 2,
      },
      {
        x: '2006',
        y: 0,
      },
      {
        x: '2010',
        y: 0,
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 2',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 1,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 3',
    data: [
      {
        y: 1,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 4',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 5',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 6',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 1,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 7',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 1,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 8',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 9',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 10',
    data: [
      {
        y: 1,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 11',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 1,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
  {
    name: 'Series 12',
    data: [
      {
        y: 0,
        x: '2000',
      },
      {
        y: 0,
        x: '2003',
      },
      {
        y: 0,
        x: '2006',
      },
      {
        y: 0,
        x: '2010',
      },
      {
        y: 0,
        x: '2016',
      },
    ],
  },
];

function HeatMap() {
  // console.log(getApexSeriesFromMockJson())
  const seri = useMemo(() => getApexSeriesFromMockJson(), []);

  console.log(seri[0]);

  return (
    <div>
      <Chart type="heatmap" series={seri} options={options} height={300} />
    </div>
  );
}

export default HeatMap;
