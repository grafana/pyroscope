import { screen } from '@testing-library/dom';
import { render } from '@testing-library/react';
import React, { useState } from 'react';

import FlameGraph from './FlameGraph';
import { getTooltipData } from './FlameGraphTooltip';
import { data } from '../data';
import { MutableDataFrame } from '@grafana/data';
import 'jest-canvas-mock';

describe('FlameGraph', () => {
  const FlameGraphWithProps = () => {
    const [topLevelIndex, setTopLevelIndex] = useState(0);
    const [rangeMin, setRangeMin] = useState(0);
    const [rangeMax, setRangeMax] = useState(1);
    const [query] = useState('');

    const flameGraphData = new MutableDataFrame({
      name: 'flamegraph',
      fields: [{ name: 'levels', values: data.flamebearer.levels.map((l) => JSON.stringify(l)) }],
    });
    flameGraphData.meta = {
      custom: {
        Names: data.flamebearer.names,
        Total: data.flamebearer.numTicks,
      },
    };

    return (
      <FlameGraph
        data={flameGraphData}
        topLevelIndex={topLevelIndex}
        rangeMin={rangeMin}
        rangeMax={rangeMax}
        query={query}
        setTopLevelIndex={setTopLevelIndex}
        setRangeMin={setRangeMin}
        setRangeMax={setRangeMax}
      />
    );
  };

  it('should render without error', async () => {
    expect(() => render(<FlameGraphWithProps />)).not.toThrow();
  });

  it('should render correctly', async () => {
    Object.defineProperty(HTMLCanvasElement.prototype, 'clientWidth', { value: 1600 });
    render(<FlameGraphWithProps />);
    
    const canvas = screen.getByTestId("flamegraph") as HTMLCanvasElement;
    const ctx = canvas!.getContext('2d');
    const calls = ctx!.__getDrawCalls();
    expect(calls).toMatchSnapshot();
  });

  describe('should get tooltip data correctly', () => {
    it('for bytes', () => {
      const tooltipData = getTooltipData(
        'memory:alloc_space:bytes:space:bytes', 
        data.flamebearer.names,
        data.flamebearer.levels,
        data.flamebearer.numTicks,
        0,
        0,
      );
      expect(tooltipData).toEqual({
        name: 'total',
        percentTitle: '% of total RAM',
        percentValue: 100,
        unitTitle: 'RAM',
        unitValue: '8.03 GB',
        samples: '8,624,078,250'
      });
    });

    it('for objects', () => {
      const tooltipData = getTooltipData(
        'memory:alloc_objects:count:space:bytes', 
        data.flamebearer.names,
        data.flamebearer.levels, 
        data.flamebearer.numTicks,
        0,
        0
      );
      expect(tooltipData).toEqual({
        name: 'total',
        percentTitle: '% of total objects',
        percentValue: 100,
        unitTitle: 'Allocated objects',
        unitValue: '8.62 G',
        samples: '8,624,078,250'
      });
    });

    it('for nanoseconds', () => {
      const tooltipData = getTooltipData(
        'process_cpu:cpu:nanoseconds:cpu:nanoseconds', 
        data.flamebearer.names, 
        data.flamebearer.levels, 
        data.flamebearer.numTicks,
        0,
        0
      );
      expect(tooltipData).toEqual({
        name: 'total',
        percentTitle: '% of total time',
        percentValue: 100,
        unitTitle: 'Time',
        unitValue: '8.62 seconds',
        samples: '8,624,078,250'
      });
    });
  });
});
