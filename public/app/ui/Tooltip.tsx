import React from 'react';
import MuiTooltip from '@mui/material/Tooltip';
import styles from './Tooltip.module.scss';

// Don't expose all props from the lib
type AvailableProps = Pick<
  React.ComponentProps<typeof MuiTooltip>,
  'title' | 'children' | 'placement'
>;
function Tooltip(props: AvailableProps) {
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

export { Tooltip };
