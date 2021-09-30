import React from 'react';
import { colorFromPercentage } from './color';

export default function FlamegraphDiffLegend() {
  const values = [100, 80, 60, 40, 20, 10, 0, -10, -20, -40, -60, -80, -100];

  return (
    <div className="row flamegraph-legend">
      <div className="flamegraph-legend-list">
        {values.map((v) => (
          <div
            key={v}
            className="flamegraph-legend-item"
            style={{
              backgroundColor: colorFromPercentage(v, 0.8),
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
