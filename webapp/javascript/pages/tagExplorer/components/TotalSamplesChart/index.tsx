import React, { useMemo } from 'react';
import { TimelineGroupData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import PieChart, { PieChartDataItem } from './PieChart';
import { calculateTotal } from '../../../math';

interface TotalSamplesChartProps {
  filteredGroupsData: TimelineGroupData[];
}

const MAX_TOP_SLICES = 5;

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

  const sortedData = pieChartData.sort((a, b) => b.data - a.data);

  const topN = sortedData.slice(0, MAX_TOP_SLICES);
  const rest = sortedData.slice(MAX_TOP_SLICES, sortedData.length);

  const final: PieChartDataItem[] = [
    ...topN,
    rest.reduce(
      (acc, current) => {
        return {
          ...acc,
          data: acc.data + current.data,
        };
      },
      {
        label: 'Other',
        color: 'grey',
        data: 0,
      }
    ),
  ];

  return (
    <PieChart
      data={final}
      id="total-samples-chart"
      height="320px"
      width="320px"
    />
  );
};

export default TotalSamplesChart;
