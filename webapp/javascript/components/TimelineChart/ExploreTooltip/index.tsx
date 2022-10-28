import React, { FC, useMemo } from 'react';
import Color from 'color';
import { getFormatter } from '@pyroscope/flamegraph/src/format/format';
import { Profile } from '@pyroscope/models/src';
import styles from './styles.module.scss';

export interface ExploreTooltipProps {
  timeLabel?: string;
  values?: Array<{
    closest: number[];
    color: number[];
    tagName: string;
  }>;
  profile?: Profile;
  coordsToCanvasPos?: jquery.flot.axis['p2c'];
  canvasX?: number;
}

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
      return `${formatter.format(v, sampleRate)} (${((v / total) * 100).toFixed(
        2
      )}%)`;
    }

    return 0;
  };

  return (
    <div>
      <div className={styles.time}>{timeLabel}</div>
      {values?.length
        ? values.map((v) => {
            return (
              <div key={v?.tagName} className={styles.valueWrapper}>
                <div
                  className={styles.valueColor}
                  style={{
                    backgroundColor: Color.rgb(v.color).toString(),
                  }}
                />
                <div>{v.tagName}:</div>
                <div className={styles.closest}>
                  {formatValue(v?.closest?.[1] || 0)}
                </div>
              </div>
            );
          })
        : null}
    </div>
  );
};

export default ExploreTooltip;
