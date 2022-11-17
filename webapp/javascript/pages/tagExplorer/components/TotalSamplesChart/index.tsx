import React, { useMemo } from 'react';
import { TimelineGroupData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { getFormatter } from '@pyroscope/flamegraph/src/format/format';
import { Profile } from '@pyroscope/models/src';
import PieChart, { PieChartDataItem } from './PieChart';
import PieChartTooltip from './PieChartTooltip';
import { calculateTotal } from '../../../math';
import { formatValue } from '../../../formatTableData';

interface TotalSamplesChartProps {
  filteredGroupsData: TimelineGroupData[];
  profile?: Profile;
  formatter?: ReturnType<typeof getFormatter>;
}

const TotalSamplesChart = ({
  filteredGroupsData,
  formatter,
  profile,
}: TotalSamplesChartProps) => {
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
      height="280px"
      width="280px"
      onHoverTooltip={(data) => (
        <PieChartTooltip
          label={data.label}
          value={formatValue({ formatter, profile, value: data.value })}
          percent={data.percent}
        />
      )}
    />
  );
};

export default TotalSamplesChart;
