import React, { useMemo } from 'react';
import { TimelineGroupData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import PieChart, { PieChartDataItem } from './PieChart';
import PieChartTooltip, { PieChartTooltipProps } from './PieChartTooltip';
import { calculateTotal } from '../../../math';

interface TotalSamplesChartProps {
  filteredGroupsData: TimelineGroupData[];
}

const TotalSamplesChart = ({ filteredGroupsData }: TotalSamplesChartProps) => {
  const pieChartData: PieChartDataItem[] = useMemo(() => {
    return filteredGroupsData.length
      ? filteredGroupsData.map((d) => ({
          label: d.tagName,
          data: calculateTotal(d.data.samples),
          color: d.color,
        }))
      : [];
  }, [filteredGroupsData]);

  return (
    <PieChart
      data={pieChartData}
      id="total-samples-chart"
      height="220px"
      width="220px"
      onHoverTooltip={(data: PieChartTooltipProps) => (
        <PieChartTooltip
          label={data.label}
          value={data.value}
          percent={data.percent}
        />
      )}
    />
  );
};

export default TotalSamplesChart;
