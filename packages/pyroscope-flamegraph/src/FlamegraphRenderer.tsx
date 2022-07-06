import React from 'react';
import FlameGraphRenderer from './FlameGraph/FlameGraphRenderer';
import '@pyroscope/webapp/sass/flamegraph.scss';

const overrideProps = {
  //  showPyroscopeLogo: !process.env.PYROSCOPE_HIDE_LOGO as any, // this is injected by webpack
  showPyroscopeLogo: false,
};

export type FlamegraphRendererProps = Omit<
  FlameGraphRenderer['props'],
  'showPyroscopeLogo'
>;

// TODO: type props
export const FlamegraphRenderer = (props: FlamegraphRendererProps) => {
  // eslint-disable-next-line react/jsx-props-no-spreading
  return <FlameGraphRenderer {...props} {...overrideProps} />;
};
