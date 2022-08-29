import React, { useState } from 'react';
import { DataFrame } from '@grafana/data';

import FlameGraphHeader from './FlameGraphHeader';
import FlameGraph from './FlameGraph';

type Props = {
  data: DataFrame;
};

const FlameGraphContainer = (props: Props) => {
  const [topLevelIndex, setTopLevelIndex] = useState(0);
  const [rangeMin, setRangeMin] = useState(0);
  const [rangeMax, setRangeMax] = useState(1);
  const [query, setQuery] = useState('');

  return (
    <>
      <FlameGraphHeader
        setTopLevelIndex={setTopLevelIndex}
        setRangeMin={setRangeMin}
        setRangeMax={setRangeMax}
        query={query}
        setQuery={setQuery}
      />

      <FlameGraph
        data={props.data}
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

export default FlameGraphContainer;
