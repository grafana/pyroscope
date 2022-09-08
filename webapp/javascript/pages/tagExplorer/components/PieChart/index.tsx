import React from 'react';
import ReactFlot from 'react-flot';
import { TimelineGroupData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import styles from './styles.module.scss';
import { calculateMean } from '../../../math';

const PieChart = ({ data }: { data: TimelineGroupData[] }) => {
  const options = {
    series: {
      pie: {
        show: true,
        radius: 1,
        stroke: {
          width: 1,
          color: 'var(--ps-ui-foreground)',
        },
        label: {
          show: true,
          radius: 0.7,
          formatter: (name: string) => name,
          threshold: 0,
        },
      },
    },
    legend: {
      show: false,
    },
  };

  const chartData = data.length
    ? data.map((d) => ({
        label: d.tagName,
        data: calculateMean(d.data.samples),
        color: d.color,
      }))
    : [];

  if (!chartData.length) return null;

  return (
    <div className={styles.wrapper}>
      <ReactFlot
        id="product-chart"
        options={options}
        data={chartData}
        width="320px"
        height="320px"
      />
    </div>
  );
};

export default PieChart;
