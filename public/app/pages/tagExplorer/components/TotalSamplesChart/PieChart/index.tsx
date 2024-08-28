import React from 'react';
import ReactFlot from 'react-flot';
import Color from 'color';
import TooltipWrapper, {
  TooltipWrapperProps,
} from '@pyroscope/components/TimelineChart/TooltipWrapper';
import styles from './styles.module.scss';
import 'react-flot/flot/jquery.flot.pie';
import './Interactivity.plugin';

export type PieChartDataItem = {
  label: string;
  data: number;
  color: Color | string | undefined;
};

interface TooltipProps {
  label?: string;
  percent?: number;
  value?: number;
}

export interface PieChartProps {
  data: PieChartDataItem[];
  width: string;
  height: string;
  id: string;
  onHoverTooltip?: React.FC<TooltipProps>;
}

const setOnHoverDisplayTooltip = (
  data: TooltipProps & TooltipWrapperProps,
  onHoverTooltip: React.FC<TooltipProps>
) => {
  const TooltipBody = onHoverTooltip;

  if (TooltipBody) {
    return (
      <TooltipWrapper align={data.align} pageY={data.pageY} pageX={data.pageX}>
        <TooltipBody
          value={data.value}
          label={data.label}
          percent={data.percent}
        />
      </TooltipWrapper>
    );
  }

  return null;
};

const PieChart = ({
  data,
  width,
  height,
  id,
  onHoverTooltip,
}: PieChartProps) => {
  const options = {
    series: {
      pie: {
        show: true,
        radius: 1,
        stroke: {
          width: 0,
        },
        label: {
          show: true,
          radius: 0.7,
          threshold: 0.05,
          formatter: (_: string, data: { percent: number }) =>
            `${data.percent.toFixed(2)}%`,
        },
      },
    },
    legend: {
      show: false,
    },
    grid: {
      hoverable: true,
      clickable: false,
    },
    pieChartTooltip: onHoverTooltip
      ? (tooltipData: TooltipProps & TooltipWrapperProps) =>
          setOnHoverDisplayTooltip(tooltipData, onHoverTooltip)
      : null,
  };

  return (
    <div className={styles.wrapper}>
      <ReactFlot
        id={id}
        options={options}
        data={data}
        width={width}
        height={height}
      />
    </div>
  );
};

export default PieChart;
