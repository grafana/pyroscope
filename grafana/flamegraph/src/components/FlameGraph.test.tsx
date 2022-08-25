import { screen } from '@testing-library/dom';
import { act, render } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React, { useState } from 'react';

import FlameGraph from './FlameGraph';
import { data } from '../data';
import { MutableDataFrame } from '@grafana/data';

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

  it('should render children', async () => {
    Object.defineProperty(HTMLDivElement.prototype, 'clientWidth', { value: 1600 });
    render(<FlameGraphWithProps />);

    // first bar
    expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
    expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');

    // first bar click goes nowhere
    await userEvent.click(screen.getByTestId('flamegraph').children[0]);
    expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
    expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');

    // second bar has no text (below label threshold)
    expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual('');

    // second bar click (focuses second bar and shows every bar above itself)
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[1]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(41);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual(
        'github.com/grafana/fire/pkg/distributor.(*Distributor).Push.func1'
      );
      expect(screen.getByTestId('flamegraph').children[35].textContent).toEqual('sync.(*Pool).Get');
    });

    // reset to show all bars
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[0]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
    });

    // fourth bar click
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[0]); // reset to show all bars
      expect(screen.getByTestId('flamegraph').children[3].textContent).toEqual(
        'github.com/grafana/fire/pkg/agent.(*Target).start.func1'
      );
      await userEvent.click(screen.getByTestId('flamegraph').children[3]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(31);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual(
        'github.com/grafana/fire/pkg/agent.(*Target).start.func1'
      );
    });

    // greyed out bar click that has no data-x, data-y attributes (bar is collapsed) so click goes nowhere
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[0]);
      expect(screen.getByTestId('flamegraph').children[41].textContent).toEqual('');
      await userEvent.click(screen.getByTestId('flamegraph').children[41]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[2].textContent).toEqual('net/http.(*conn).serve');
      expect(screen.getByTestId('flamegraph').children[41].textContent).toEqual('');
    });

    // clicking down the tree
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[0]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
      expect(screen.getByTestId('flamegraph').children[4].textContent).toEqual('runtime.main');
      await userEvent.click(screen.getByTestId('flamegraph').children[4]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(130);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual('runtime.main');
      expect(screen.getByTestId('flamegraph').children[7].textContent).toEqual('runtime.doInit');
      await userEvent.click(screen.getByTestId('flamegraph').children[7]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(108);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual('runtime.main');
      expect(screen.getByTestId('flamegraph').children[2].textContent).toEqual('runtime.doInit');
      expect(screen.getByTestId('flamegraph').children[3].textContent).toEqual('runtime.doInit');
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual('text/template/parse.(*Tree).Parse');
      await userEvent.click(screen.getByTestId('flamegraph').children[20]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(21);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[5].textContent).toEqual('github.com/grafana/dskit/ring.init');
      expect(screen.getByTestId('flamegraph').children[7].textContent).toEqual('text/template.(*Template).Parse');
      expect(screen.getByTestId('flamegraph').children[9].textContent).toEqual('text/template/parse.(*Tree).Parse');
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual(
        'text/template/parse.(*PipeNode).append'
      );
      await userEvent.click(screen.getByTestId('flamegraph').children[20]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(20);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[10].textContent).toEqual('text/template/parse.(*Tree).parse');
      expect(screen.getByTestId('flamegraph').children[15].textContent).toEqual('text/template/parse.(*Tree).itemList');
      expect(screen.getByTestId('flamegraph').children[18].textContent).toEqual('text/template/parse.(*Tree).pipeline');
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'text/template/parse.(*PipeNode).append'
      );
    });

    // clicking back up the tree
    await act(async () => {
      await userEvent.click(screen.getByTestId('flamegraph').children[18]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(21);
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'text/template/parse.(*Tree).newPipeline'
      );
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual(
        'text/template/parse.(*PipeNode).append'
      );
      await userEvent.click(screen.getByTestId('flamegraph').children[8]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(21);
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'text/template/parse.(*Tree).newPipeline'
      );
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual(
        'text/template/parse.(*PipeNode).append'
      );
      await userEvent.click(screen.getByTestId('flamegraph').children[4]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(108);
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'github.com/prometheus/client_golang/prometheus.(*SummaryVec).WithLabelValues'
      );
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual('text/template/parse.(*Tree).Parse');
      await userEvent.click(screen.getByTestId('flamegraph').children[3]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(108);
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'github.com/prometheus/client_golang/prometheus.(*SummaryVec).WithLabelValues'
      );
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual('text/template/parse.(*Tree).Parse');
      await userEvent.click(screen.getByTestId('flamegraph').children[1]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(130);
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual(
        'github.com/mwitkow/go-conntrack.NewDialContextFunc'
      );
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual('runtime.doInit');
      await userEvent.click(screen.getByTestId('flamegraph').children[0]);
      expect(screen.getByTestId('flamegraph').children.length).toEqual(338);
      expect(screen.getByTestId('flamegraph').children[0].textContent).toEqual('total');
      expect(screen.getByTestId('flamegraph').children[1].textContent).toEqual('');
      expect(screen.getByTestId('flamegraph').children[2].textContent).toEqual('net/http.(*conn).serve');
      expect(screen.getByTestId('flamegraph').children[19].textContent).toEqual('runtime.gcMarkDone');
      expect(screen.getByTestId('flamegraph').children[20].textContent).toEqual(
        'github.com/weaveworks/common/signals.(*Handler).Loop'
      );
    });
  });
});
