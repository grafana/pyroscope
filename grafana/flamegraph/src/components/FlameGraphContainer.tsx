import React, { useState } from 'react';

import FlameGraph from './FlameGraph';
import FlameGraphHeader from './FlameGraphHeader';
import { data } from '../data';

const FlameGraphContainer = () => {
  const flameGraphData = data['flamebearer'];
  const [topLevelIndex, setTopLevelIndex] = useState(0)
  const [rangeMin, setRangeMin] = useState(0)
  const [rangeMax, setRangeMax] = useState(1)

  return (
    <>
      <FlameGraphHeader 
        setTopLevelIndex={setTopLevelIndex} 
        setRangeMin={setRangeMin} 
        setRangeMax={setRangeMax}
      />

      <FlameGraph 
        data={flameGraphData}
        topLevelIndex={topLevelIndex}
        rangeMin={rangeMin}
        rangeMax={rangeMax}
        setTopLevelIndex={setTopLevelIndex}
        setRangeMin={setRangeMin}
        setRangeMax={setRangeMax}
      />
    </>
  );
}

export default FlameGraphContainer;
