// Layout wrapper components that will allow us to override these layout components with alternative styling

import Box from '@pyroscope/ui/Box';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import React, { ReactNode } from 'react';

export function PageContentWrapper({ children }: { children: ReactNode }) {
  return <div className="main-wrapper">{children}</div>;
}

type PanelProps = {
  isLoading: boolean;
  title?: ReactNode;
  children: ReactNode;
  className?: string;
};

/** Common pattern which wraps a chart and optional title */
export function Panel({
  isLoading,
  title,
  children,
  className = '',
}: PanelProps) {
  // If there is a title, spinnerPosition is at the baseline
  // Otherwise, it is undeclared

  const spinnerPosition = title ? 'baseline' : undefined;

  return (
    <Box className={className}>
      <LoadingOverlay active={isLoading} spinnerPosition={spinnerPosition}>
        <div style={{ background: 'magenta' }}>{title}</div>
        <div style={{ background: 'cyan' }}>{children}</div>
      </LoadingOverlay>
    </Box>
  );
}
