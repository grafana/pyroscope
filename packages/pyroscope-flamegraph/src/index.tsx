/* eslint-disable react/react-in-jsx-scope */
/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import FlameGraphRenderer from './FlameGraph/FlameGraphRenderer';
import Flamegraph from './FlameGraph/FlameGraphComponent/Flamegraph';

const overrideProps = {
  showPyroscopeLogo: !process.env.PYROSCOPE_HIDE_LOGO as any, // this is injected by webpack
};

// TODO: type props
const FlamegraphRenderer = (props: any) => {
  return <FlameGraphRenderer {...props} {...overrideProps} />;
};

export { FlamegraphRenderer, Flamegraph };
