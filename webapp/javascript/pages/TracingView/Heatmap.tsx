import React, { useState, Dispatch, SetStateAction } from 'react';
import Color from 'color';
import type { ApexOptions } from 'apexcharts';
import Chart from 'react-apexcharts';
import globalTemperatureJSON from './global-temperature.json';

const getOptions = (setClickedItemCoord: Dispatch<SetStateAction<string>>) => ({
  // to not edit cells color when hover or active (clicked)
  states: {
    hover: {
      filter: {
        type: 'none',
      },
    },
    active: {
      filter: {
        type: 'none',
      },
    },
  },
  chart: {
    toolbar: {
      show: false,
    },
    events: {
      // todo: fix types
      // any type is from lib definitions
      click: (e: any, _: any, config: any) => {
        const el = e.target;

        // handle empty cells click
        if (el.getAttribute('val') !== '0') {
          const seriesIndex = parseInt(el.getAttribute('i'));
          const dataPointIndex = parseInt(el.getAttribute('j'));

          setClickedItemCoord(`
            ${config.globals.seriesNames[seriesIndex]},
            ${config.globals.labels[dataPointIndex]}
          `);
        }
      },
    },
  },
  dataLabels: {
    enabled: false,
  },
  colors: ['#9C27B0'],
  tooltip: {
    enabled: false,
  },
  grid: {
    show: false,
  },
  useFillColorAsStroke: true,
  plotOptions: {
    heatmap: {
      radius: 0,
      useFillColorAsStroke: true,
    },
  },
  yaxis: {
    axisTicks: {
      show: true,
      // add color mode
      color: Color('grey').toString(),
      offsetX: 2,
      offsetY: 1,
    },
    axisBorder: {
      show: true,
      color: Color('grey').toString(),
      offsetX: -2,
    },
    labels: {
      // value is buckets
      formatter: (value: number): string => {
        return value ? value / 1000 + 'K' : '';
      },
      style: {
        // add color mode
        colors: Color('white').toString(),
      },
    },
  },
  xaxis: {
    tickAmount: 8,
    labels: {
      // value is timestamp
      formatter: (value: string): string => {
        return value + 'mock time';
      },
      style: {
        // add color mode
        colors: Color('white').toString(),
      },
    },
    axisTicks: {
      color: Color('grey').toString(),
      offsetX: -1,
    },
    axisBorder: {
      color: Color('grey').toString(),
      offsetX: -1,
    },
  },
});

const getApexSeriesFromMockJson = (): ApexOptions['series'] => {
  return globalTemperatureJSON.monthlyVariance.reduce(
    (acc, { year, month, variance }) => {
      if (acc[month - 1]) {
        acc[month - 1].data.push({
          x: year.toString(),
          y: (variance > 0 ? variance * 10 : 0).toString(),
        });
      } else {
        acc[month - 1] = {
          name: (month * 1000).toString(),
          data: [
            {
              x: year.toString(),
              y: (variance > 0 ? variance * 10 : 0).toString(),
            },
          ],
        };
      }
      return acc;
    },
    new Array(12)
  );
};

const oneSeries = {
  name: '8000', // bucket duration (yaxis)
  data: [
    { x: '1', y: '0' }, // x is timestamp, y is number of items in the bucket
    { x: '2', y: '10' },
    { x: '3', y: '0' },
    { x: '4', y: '15' },
    { x: '5', y: '0' },
    { x: '6', y: '20' },
  ],
};
const twoSeries = {
  name: '8500',
  data: [
    { x: '1', y: '10' },
    { x: '2', y: '20' },
    { x: '3', y: '30' },
    { x: '4', y: '55' },
    { x: '5', y: '6' },
    { x: '6', y: '20' },
  ],
};
const threeSeries = {
  name: '18500',
  data: [
    { x: '1', y: '0' },
    { x: '2', y: '0' },
    { x: '3', y: '33' },
    { x: '4', y: '11' },
    { x: '5', y: '0' },
    { x: '6', y: '10' },
  ],
};

function HeatMap() {
  const [lastClickedItem, setLastClickedItem] = useState('');

  return (
    <div>
      <Chart
        type="heatmap"
        // series={[oneSeries, twoSeries, threeSeries]}
        series={getApexSeriesFromMockJson()}
        options={getOptions(setLastClickedItem)}
        height={300}
      />
      <br />
      {lastClickedItem && <h4>last clicked item(x, y): {lastClickedItem}</h4>}
    </div>
  );
}

export default HeatMap;
