import React from 'react';

import type { TimelineGroupData } from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { appWithoutTagsWhereDropdownOptionName } from '@webapp/redux/reducers/continuous';
import styles from './Legend.module.scss';

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
      {groups.map(({ tagName, color }) => (
        <div
          aria-hidden
          data-testid="legend-item"
          className={styles.tagName}
          key={tagName}
          onClick={() =>
            handleGroupByTagValueChange(
              tagName === activeGroup
                ? appWithoutTagsWhereDropdownOptionName
                : tagName
            )
          }
        >
          <span
            data-testid="legend-item-color"
            className={styles.tagColor}
            style={{ backgroundColor: color?.toString() }}
          />
          <span>{tagName}</span>
        </div>
      ))}
    </div>
  );
}

export default Legend;
