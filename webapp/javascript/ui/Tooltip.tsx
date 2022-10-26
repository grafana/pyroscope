/* eslint-disable 
jsx-a11y/click-events-have-key-events, 
jsx-a11y/no-noninteractive-element-interactions, 
css-modules/no-unused-class 
*/
import React from 'react';
import MuiTooltip from '@mui/material/Tooltip';
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

// Don't expose all props from the lib
type AvailableProps = Pick<
  React.ComponentProps<typeof MuiTooltip>,
  'title' | 'children' | 'placement'
>;
function Tooltip2(props: AvailableProps) {
  const defaultProps: Omit<
    React.ComponentProps<typeof MuiTooltip>,
    'title' | 'children'
  > = {
    arrow: true,
    classes: {
      tooltip: styles.muiTooltip,
      arrow: styles.muiTooltipArrow,
    },
  };

  /* eslint-disable-next-line react/jsx-props-no-spreading */
  return <MuiTooltip {...defaultProps} {...props} />;
}

export default Tooltip;
export { Tooltip2 };
