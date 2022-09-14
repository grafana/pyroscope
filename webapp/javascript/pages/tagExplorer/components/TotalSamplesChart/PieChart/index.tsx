import React from 'react';
import ReactFlot from 'react-flot';
import Color from 'color';
import TooltipWrapper, {
  ITooltipWrapperProps,
} from '@webapp/components/TimelineChart/TooltipWrapper';
import { PieChartTooltipProps } from '../PieChartTooltip';
import styles from './styles.module.scss';
import 'react-flot/flot/jquery.flot.pie';
import './Interactivity.plugin';

export type PieChartDataItem = {
  label: string;
  data: number;
  color: Color | string | undefined;
};

interface PieChartProps {
  data: PieChartDataItem[];
  width: string;
  height: string;
  id: string;
  onHoverTooltip?: React.FC<PieChartTooltipProps>;
}

const setOnHoverDisplayTooltip = (
  data: PieChartTooltipProps & ITooltipWrapperProps,
  onHoverTooltip: React.FC<PieChartTooltipProps>
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
          threshold: 0.17,
          formatter: (label: string) => label,
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
      ? (tooltipData: PieChartTooltipProps & ITooltipWrapperProps) =>
          setOnHoverDisplayTooltip(tooltipData, onHoverTooltip)
      : null,
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
