import React, { useMemo, FC } from 'react';
import classNames from 'classnames/bind';
import styles from './ExploreTooltip.module.scss';

const cx = classNames.bind(styles);

const COMPONENT_WIDTH = 450;

interface ExploreTooltipProps {
  pageX: number;
  pageY: number;
  align: 'left' | 'right';
  timeLabel?: string;
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
      {props.timeLabel}
    </div>
  );
};

export default ExploreTooltip;
