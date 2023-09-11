import React from 'react';
import type { TimelineGroupData } from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import { ALL_TAGS } from '@pyroscope/redux/reducers/continuous';
import classNames from 'classnames/bind';
import styles from './Legend.module.scss';

const cx = classNames.bind(styles);
interface LegendProps {
  groups: TimelineGroupData[];
  handleGroupByTagValueChange: (groupByTagValue: string) => void;
  activeGroup: string;
}

function Legend({
  groups,
  handleGroupByTagValueChange,
  activeGroup,
}: LegendProps) {
  return (
    <div data-testid="legend" className={styles.legend}>
      {groups.map(({ tagName, color }) => {
        const isSelected = tagName === activeGroup;
        return (
          <div
            aria-hidden
            data-testid="legend-item"
            className={cx({
              [styles.tagName]: true,
              [styles.faded]: activeGroup && !isSelected,
            })}
            key={tagName}
            onClick={() =>
              handleGroupByTagValueChange(isSelected ? ALL_TAGS : tagName)
            }
          >
            <span
              data-testid="legend-item-color"
              className={styles.tagColor}
              style={{ backgroundColor: color?.toString() }}
            />
            <span>{tagName}</span>
          </div>
        );
      })}
    </div>
  );
}

export default Legend;
