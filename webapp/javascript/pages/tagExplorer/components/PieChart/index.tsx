import React from 'react';
import ReactFlot from 'react-flot';
import Color from 'color';
import styles from './styles.module.scss';

type PieChartDataItem = {
  label: string;
  data: number;
  color: Color | undefined;
};

interface PieChartProps {
  data: PieChartDataItem[];
  width: string;
  height: string;
  id: string;
}

const PieChart = ({ data, width, height, id }: PieChartProps) => {
  const options = {
    series: {
      pie: {
        show: true,
        radius: 1,
        stroke: {
          width: 0.5,
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

  return (
    <div className={styles.wrapper}>
      <ReactFlot
        id={id}
        options={options}
        data={data}
        width={height}
        height={width}
      />
    </div>
  );
};

export default PieChart;
