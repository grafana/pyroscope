import React from 'react';
import { faInfoCircle } from '@fortawesome/free-solid-svg-icons/faInfoCircle';
import Button from '@pyroscope/ui/Button';
import styles from './TooltipInfoIcon.module.scss';

export const TooltipInfoIcon = React.forwardRef(function TooltipInfoIcon(
  props,
  ref: React.ComponentProps<typeof Button>['ref']
) {
  return (
    <Button
      // needed for tooltip
      // eslint-disable-next-line react/jsx-props-no-spreading
      {...props}
      ref={ref}
      className={styles.noHover}
      icon={faInfoCircle}
      kind="float"
    />
  );
});
