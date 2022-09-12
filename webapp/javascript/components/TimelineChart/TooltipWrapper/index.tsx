import React, { useMemo } from 'react';
import classNames from 'classnames/bind';
import styles from './styles.module.scss';

const cx = classNames.bind(styles);

const EXPLORE_TOOLTIP_WRAPPER_ID = 'explore_tooltip_wrapper';
const DEFAULT_COMPONENT_WIDTH = 450;

export interface ITooltipWrapperProps {
  pageX: number;
  pageY: number;
  align: 'left' | 'right';
  children: React.ReactNode | React.ReactNode[];
}

const TooltipWrapper = ({
  pageX,
  pageY,
  align,
  children,
}: ITooltipWrapperProps) => {
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
      {children}
    </div>
  );
};

export default TooltipWrapper;
