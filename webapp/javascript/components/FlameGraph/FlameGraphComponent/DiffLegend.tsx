import React from 'react';
import useResizeObserver from '@react-hook/resize-observer';
import { colorFromPercentage } from './color';
import styles from './DiffLegend.module.css';

export default function DiffLegend() {
  const legendRef = React.useRef();
  const showMode = useSizeMode(legendRef);
  const values = decideLegend(showMode);

  return (
    <div
      className={`row ${styles['flamegraph-legend']}`}
      data-testid="flamegraph-legend"
      ref={legendRef}
    >
      <div className={styles['flamegraph-legend-list']}>
        {values.map((v) => (
          <div
            key={v}
            className={styles['flamegraph-legend-item']}
            style={{
              backgroundColor: colorFromPercentage(v, 0.8).string(),
            }}
          >
            {v > 0 ? '+' : ''}
            {v}%
          </div>
        ))}
      </div>
    </div>
  );
}

function decideLegend(showMode: ReturnType<typeof useSizeMode>) {
  switch (showMode) {
    case 'large': {
      return [100, 80, 60, 40, 20, 10, 0, -10, -20, -40, -60, -80, -100];
    }

    case 'small': {
      return [100, 40, 20, 0, -20, -40, -100];
    }

    default:
      throw new Error(`Unsupported ${showMode}`);
  }
}

/**
 * TODO: unify this and toolbar's
 * Custom hook that returns the size ('large' | 'small')
 * that should be displayed
 * based on the toolbar width
 */
// arbitrary value
// as a simple heuristic, try to run the comparison view
// and see when the buttons start to overlap
const WIDTH_THRESHOLD = 600;
const useSizeMode = (target: React.RefObject<HTMLDivElement>) => {
  const [size, setSize] = React.useState<'large' | 'small'>('large');

  const calcMode = (width: number) => {
    if (width < WIDTH_THRESHOLD) {
      return 'small';
    }
    return 'large';
  };

  React.useLayoutEffect(() => {
    if (target.current) {
      const { width } = target.current.getBoundingClientRect();

      setSize(calcMode(width));
    }
  }, [target.current]);

  useResizeObserver(target, (entry: ResizeObserverEntry) => {
    setSize(calcMode(entry.contentRect.width));
  });

  return size;
};
