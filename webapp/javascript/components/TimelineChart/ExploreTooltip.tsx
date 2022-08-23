import React, { useMemo, FC } from 'react';
import classNames from 'classnames/bind';
import Color from 'color';
import styles from './ExploreTooltip.module.scss';

const cx = classNames.bind(styles);

const DEFAULT_COMPONENT_WIDTH = 450;
const EXPLORE_TOOLTIP_WRAPPER_ID = 'explore_tooltip_wrapper';

interface ExploreTooltipProps {
  pageX: number;
  pageY: number;
  align: 'left' | 'right';
  timeLabel?: string;
  values?: Array<{
    closest: number[];
    color: number[];
    tagName: string;
  }>;
}

const ExploreTooltip: FC<ExploreTooltipProps> = ({
  pageX,
  pageY,
  align,
  timeLabel,
  values,
}) => {
  const isHidden = useMemo(() => pageX < 0 || pageY < 0, [pageX, pageY]);

  const left = useMemo(() => {
    if (!isHidden) {
      const elem = document.getElementById(
        EXPLORE_TOOLTIP_WRAPPER_ID
      )?.offsetWidth;

      return align === 'right'
        ? pageX + 20
        : pageX - (elem || DEFAULT_COMPONENT_WIDTH) - 20;
    }
    return -1;
  }, [align, pageX, isHidden]);

  return (
    <div
      style={{ top: pageY, left }}
      className={cx({
        [styles.tooltip]: true,
        [styles.hidden]: isHidden,
      })}
      id={EXPLORE_TOOLTIP_WRAPPER_ID}
    >
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
                <div>{v?.tagName}:</div>
                <div className={styles.closest}>{v?.closest?.[1] || '0'}</div>
              </div>
            );
          })
        : null}
    </div>
  );
};

export default ExploreTooltip;
