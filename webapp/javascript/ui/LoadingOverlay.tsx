import React from 'react';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import cx from 'classnames';
import styles from './LoadingOverlay.module.css';

/**
 * LoadingOverlay when 'active' will cover the entire parent's width/height with
 * an overlay and a loading spinner
 */
export function LoadingOverlay({
  active = true,
  spinnerPosition = 'center',
  kind = 'blur',
}: {
  spinnerPosition?: 'center' | 'baseline';
  active?: boolean;
  kind?: 'dark' | 'blur';
}) {
  if (!active) {
    return null;
  }

  // TODO(eh-am): wait few ms before displaying
  // so that if the request is fast enough we don't show anything
  return (
    <div
      className={cx(
        styles.loadingOverlay,
        kind === 'dark' ? styles.withDarkoverlay : styles.withBlurOverlay
      )}
      style={{
        alignItems: spinnerPosition,
        zIndex: 99,
      }}
    >
      <LoadingSpinner size="46px" />
    </div>
  );
}
