import React, { useState } from 'react';
import { DataFrame } from '@grafana/data';

import FlameGraph from './FlameGraph';
import FlameGraphHeader from './FlameGraphHeader';

type Props = {
  data: DataFrame;
};

const FlameGraphContainer = (props: Props) => {
  const [topLevelIndex, setTopLevelIndex] = useState(0);
  const [rangeMin, setRangeMin] = useState(0);
  const [rangeMax, setRangeMax] = useState(1);

  return (
    <>
      <FlameGraphHeader setTopLevelIndex={setTopLevelIndex} setRangeMin={setRangeMin} setRangeMax={setRangeMax} />

      <FlameGraph
        data={props.data}
        topLevelIndex={topLevelIndex}
        rangeMin={rangeMin}
        rangeMax={rangeMax}
        setTopLevelIndex={setTopLevelIndex}
        setRangeMin={setRangeMin}
        setRangeMax={setRangeMax}
      />
    </>
  );
};

export default FlameGraphContainer;
