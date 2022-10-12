import React, { useEffect, useState } from 'react';
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

/**
 * LoadingOverlay when 'active' will cover the entire parent's width/height with
 * an overlay and a loading spinner
 */
export function LoadingOverlay2({
  active = true,
  spinnerPosition = 'center',
  kind = 'dark',
  children,
  delay = 250,
}: {
  /** where to position the spinner. use baseline when the component's vertical center is outside the viewport */
  spinnerPosition?: 'center' | 'baseline';
  active?: boolean;
  kind?: 'dark' | 'blur';
  children?: React.ReactNode;
  /** delay in ms before showing the overlay. this evicts a flick */
  delay: number;
}) {
  // Wait for `delay`
  // https://www.nngroup.com/articles/progress-indicators/
  const [isVisible, setVisible] = useState(false);
  useEffect(() => {
    if (active) {
      const timeoutID = window.setTimeout(() => {
        setVisible(true);
      }, delay);

      return () => clearTimeout(timeoutID);
    }

    setVisible(false);
  }, [active]);

  return (
    <div>
      <div
        className={cx(
          styles.loadingOverlay,
          styles.withDarkoverlay,
          !isVisible ? styles.unactive : null
        )}
        style={{
          alignItems: spinnerPosition,
        }}
      >
        <LoadingSpinner size="46px" />
      </div>

      {children}
    </div>
  );
}
