import React, { FC, useMemo } from 'react';
import Color from 'color';
import { getFormatter } from '@pyroscope/legacy/flamegraph/format/format';
import { Profile } from '@pyroscope/legacy/models';
import { TimelineTooltip } from '../../TimelineTooltip';
import { TooltipCallbackProps } from '../Tooltip.plugin';

type ExploreTooltipProps = TooltipCallbackProps & {
  profile?: Profile;
};

const ExploreTooltip: FC<ExploreTooltipProps> = ({
  timeLabel,
  values,
  profile,
}) => {
  const numTicks = profile?.flamebearer?.numTicks;
  const sampleRate = profile?.metadata?.sampleRate;
  const units = profile?.metadata?.units;

  const formatter = useMemo(
    () =>
      numTicks &&
      typeof sampleRate === 'number' &&
      units &&
      getFormatter(numTicks, sampleRate, units),
    [numTicks, sampleRate, units]
  );

  const total = useMemo(
    () =>
      values?.length
        ? values?.reduce((acc, current) => acc + (current.closest?.[1] || 0), 0)
        : 0,
    [values]
  );

  const formatValue = (v: number) => {
    if (formatter && typeof sampleRate === 'number') {
      const value = formatter.format(v, sampleRate);
      let percentage = (v / total) * 100;

      if (Number.isNaN(percentage)) {
        percentage = 0;
      }

      return `${value} (${percentage.toFixed(2)}%)`;
    }

    return '0';
  };

  const items = values.map((v) => {
    return {
      label: v.tagName || '',
      color: Color.rgb(v.color),
      value: formatValue(v?.closest?.[1] || 0),
    };
  });

  return <TimelineTooltip timeLabel={timeLabel} items={items} />;
};

export default ExploreTooltip;
