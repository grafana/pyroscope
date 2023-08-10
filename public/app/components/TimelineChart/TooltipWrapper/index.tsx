import React, { useMemo } from 'react';
import classNames from 'classnames/bind';
import styles from './styles.module.scss';

const cx = classNames.bind(styles);

const EXPLORE_TOOLTIP_WRAPPER_ID = 'explore_tooltip_wrapper';

export interface TooltipWrapperProps {
  pageX: number;
  pageY: number;
  align: 'left' | 'right';
  children: React.ReactNode | React.ReactNode[];
  className?: string;
}

const TooltipWrapper = ({
  className,
  pageX,
  pageY,
  align,
  children,
}: TooltipWrapperProps) => {
  const isHidden = useMemo(() => pageX < 0 || pageY < 0, [pageX, pageY]);

  const style =
    align === 'right'
      ? { top: pageY, left: pageX + 20, right: 'auto' }
      : { top: pageY, left: 'auto', right: window.innerWidth - (pageX - 20) };

  return (
    <div
      style={style}
      className={cx({
        [styles.tooltip]: true,
        [styles.hidden]: isHidden,
        [className || '']: className,
      })}
      id={EXPLORE_TOOLTIP_WRAPPER_ID}
    >
      {children}
    </div>
  );
};

export default TooltipWrapper;
