import Box from '@pyroscope/ui/Box';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import React, { ReactNode } from 'react';

import styles from './Panel.module.css';

export type PanelProps = {
  isLoading: boolean;
  title?: ReactNode;
  children: ReactNode;
  className?: string;
  headerActions?: ReactNode;
  dataTestId?: string;
};

/** Common pattern which wraps a chart and optional title */
export function Panel({
  isLoading,
  title,
  children,
  className = '',
  headerActions,
}: PanelProps) {
  // If there is a title, spinnerPosition is at the baseline
  // Otherwise, it is undeclared

  const spinnerPosition = title ? 'baseline' : undefined;

  return (
    <Box className={className}>
      <LoadingOverlay active={isLoading} spinnerPosition={spinnerPosition}>
        {(title || headerActions) && (
          <div className={styles.panelTitleWrapper}>
            {title} {headerActions}
          </div>
        )}
        <div>{children}</div>
      </LoadingOverlay>
    </Box>
  );
}
