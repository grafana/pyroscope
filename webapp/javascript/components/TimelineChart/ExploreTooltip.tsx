import React, { useMemo, FC } from 'react';
import classNames from 'classnames/bind';
import Color from 'color';
import styles from './ExploreTooltip.module.scss';

const cx = classNames.bind(styles);

const COMPONENT_WIDTH = 150;

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

const ExploreTooltip: FC<ExploreTooltipProps> = (props) => {
  const isHidden = useMemo(
    () => props.pageX < 0 || props.pageY < 0,
    [props.pageX, props.pageY]
  );

  const left = useMemo(
    () =>
      props.align === 'right'
        ? props.pageX + 20
        : props.pageX - COMPONENT_WIDTH - 20,
    [props.align, props.pageX]
  );

  return (
    <div
      style={{
        width: COMPONENT_WIDTH,
        top: props.pageY,
        left,
      }}
      className={cx({
        [styles.tooltip]: true,
        [styles.hidden]: isHidden,
      })}
    >
      <div className={styles.time}>{props.timeLabel}</div>
      {props.values?.length
        ? props.values.map((v) => {
            return (
              <div key={v?.tagName} className={styles.valueWrapper}>
                <div
                  className={styles.valueColor}
                  style={{
                    backgroundColor: Color.rgb(v.color).toString(),
                  }}
                />
                <div>{v?.closest?.[1] || '0'}</div>
              </div>
            );
          })
        : null}
    </div>
  );
};

export default ExploreTooltip;
