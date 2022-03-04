/* eslint-disable react/react-in-jsx-scope */
/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: Until we rewrite FlamegraphRenderer in typescript this will do
import FlameGraphRenderer from './FlameGraph/FlameGraphRenderer';
import Flamegraph from './FlameGraph/FlameGraphComponent/Flamegraph';

const overrideProps = {
  //  showPyroscopeLogo: !process.env.PYROSCOPE_HIDE_LOGO as any, // this is injected by webpack
  showPyroscopeLogo: false,
};

// TODO: type props
const FlamegraphRenderer = (props: any) => {
  return <FlameGraphRenderer {...props} {...overrideProps} />;
};

export { FlamegraphRenderer, Flamegraph };
