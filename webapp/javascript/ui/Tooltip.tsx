/* eslint-disable 
jsx-a11y/click-events-have-key-events, 
jsx-a11y/no-noninteractive-element-interactions, 
css-modules/no-unused-class 
*/
import React from 'react';
import styles from './Tooltip.module.scss';

interface TooltipProps {
  title: string;
  visible: boolean;
  className?: string;
  placement: 'top' | 'left' | 'bottom' | 'right';
}

const Tooltip = ({ title, visible, className, placement }: TooltipProps) => {
  return (
    <div
      onClick={(e) => e.stopPropagation()}
      className={`${styles.tooltip} ${visible ? styles.visible : ''} ${
        styles?.[placement]
      } ${className || ''} `}
      role="tooltip"
    >
      {title}
    </div>
  );
};

export default Tooltip;
