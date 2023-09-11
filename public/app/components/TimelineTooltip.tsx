import React from 'react';
import Color from 'color';
import styles from './TimelineTooltip.module.css';

export interface TimelineTooltipProps {
  timeLabel: string;
  items: Array<{
    color?: Color;
    value: string;
    label: string;
  }>;
}

// TimelineTooltip is a generic tooltip to be used with the timeline
// It contains no logic and render items as they are
// Any formatting should be performed by the caller
function TimelineTooltip({ timeLabel, items }: TimelineTooltipProps) {
  return (
    <div>
      <div className={styles.time}>{timeLabel}</div>
      {items.map((a) => (
        <TimelineTooltipItem
          key={`${a.label}-${a.value}`}
          color={a.color}
          label={a.label}
          value={a.value}
        />
      ))}
    </div>
  );
}

function TimelineTooltipItem({
  color,
  label,
  value,
}: TimelineTooltipProps['items'][number]) {
  const ColorDiv = color ? (
    <div
      className={styles.valueColor}
      style={{ backgroundColor: Color.rgb(color).toString() }}
    />
  ) : null;

  return (
    <div className={styles.valueWrapper}>
      {ColorDiv}
      <div>{label}:</div>
      <div className={styles.closest}>{value}</div>
    </div>
  );
}

export { TimelineTooltip };
