import { screen } from '@testing-library/dom';
import { render, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React, { useState } from 'react';

import FlameGraph from './FlameGraph';
import FlameGraphHeader from './FlameGraphHeader';
import { data } from '../data';
import { MutableDataFrame } from '@grafana/data';

describe('FlameGraphContainer', () => {
  const FlameGraphContainerWithProps = () => {
    const [topLevelIndex, setTopLevelIndex] = useState(0);
    const [rangeMin, setRangeMin] = useState(0);
    const [rangeMax, setRangeMax] = useState(1);
    const [query, setQuery] = useState('');

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
      <>
        <FlameGraphHeader
          query={query}
          setQuery={setQuery}
          setTopLevelIndex={setTopLevelIndex}
          setRangeMin={setRangeMin}
          setRangeMax={setRangeMax}
        />

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
      </>
    );
  };

  it('search should highlight matching results', async () => {
    Object.defineProperty(HTMLDivElement.prototype, 'clientWidth', { value: 1600 });
    render(<FlameGraphContainerWithProps />);

    expect(screen.getByTestId('flamegraph').children[0].getAttribute('style')).toContain(
      'background: rgb(255, 112, 112)'
    );
    expect(screen.getByTestId('flamegraph').children[2].getAttribute('style')).toContain('background: rgb(87, 87, 87)');
    expect(screen.getByTestId('flamegraph').children[4].getAttribute('style')).toContain('background: rgb(92, 92, 92)');
    expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
    expect(screen.getByTestId('flamegraph').children[2].textContent).toEqual('net/http.(*conn).serve');
    expect(screen.getByTestId('flamegraph').children[4].textContent).toEqual('runtime.main');

    await userEvent.type(screen.getByPlaceholderText('Search..'), 'serve');
    expect(screen.getByTestId('flamegraph').children[0].getAttribute('style')).toContain(
      'background: rgb(222, 218, 247)'
    ); // greyed out as not a result
    expect(screen.getByTestId('flamegraph').children[2].getAttribute('style')).toContain('background: rgb(87, 87, 87)'); // retains colors as it is a result
    expect(screen.getByTestId('flamegraph').children[4].getAttribute('style')).toContain(
      'background: rgb(222, 218, 247)'
    ); // greyed out as not a result

    await waitFor(() => screen.getByRole('button', { name: /Reset/i }).click());
    expect(screen.getByTestId('flamegraph').children[0].getAttribute('style')).toContain(
      'background: rgb(255, 112, 112)'
    );
    expect(screen.getByTestId('flamegraph').children[2].getAttribute('style')).toContain('background: rgb(87, 87, 87)');
    expect(screen.getByTestId('flamegraph').children[4].getAttribute('style')).toContain('background: rgb(92, 92, 92)');
  });
});
