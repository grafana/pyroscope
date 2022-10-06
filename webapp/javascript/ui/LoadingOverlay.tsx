import React from 'react';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import styles from './LoadingOverlay.module.css';

/**
 * LoadingOverlay when 'active' will cover the entire parent's width/height with
 * an overlay and a loading spinner
 */
export function LoadingOverlay({
  active = true,
  spinnerPosition = 'center',
}: {
  spinnerPosition?: 'center' | 'baseline';
  active?: boolean;
}) {
  if (!active) {
    return null;
  }

  // TODO(eh-am): wait few ms before displaying
  // so that if the request is fast enough we don't show anything
  return (
    <div
      className={styles.loadingOverlay}
      style={{
        alignItems: spinnerPosition,
        zIndex: 99,
      }}
    >
      <LoadingSpinner size="46px" />
    </div>
  );
}
