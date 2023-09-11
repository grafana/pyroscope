import React, { ReactNode, useEffect, useState } from 'react';
import LoadingSpinner from '@pyroscope/ui/LoadingSpinner';
import cx from 'classnames';
import styles from './LoadingOverlay.module.css';

/**
 * LoadingOverlay when 'active' will cover the entire parent's width/height with
 * an overlay and a loading spinner
 */
export function LoadingOverlay({
  active = true,
  spinnerPosition = 'center',
  children,
  delay = 250,
}: {
  /** where to position the spinner. use baseline when the component's vertical center is outside the viewport */
  spinnerPosition?: 'center' | 'baseline';
  active: boolean;
  children?: ReactNode;
  /** delay in ms before showing the overlay. this evicts a flick */
  delay?: number;
}) {
  const [isVisible, setVisible] = useState(false);

  // Wait for `delay` ms before showing the overlay
  // So that it feels snappy when server is fast
  // https://www.nngroup.com/articles/progress-indicators/
  useEffect(() => {
    if (active) {
      const timeoutID = window.setTimeout(() => {
        setVisible(true);
      }, delay);

      return () => clearTimeout(timeoutID);
    }

    setVisible(false);
    return () => {};
  }, [active, delay]);

  return (
    <div>
      <div
        className={cx(
          styles.loadingOverlay,
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
