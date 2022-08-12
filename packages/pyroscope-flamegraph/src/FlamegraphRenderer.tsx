import React from 'react';
import FlameGraphRenderer from './FlameGraph/FlameGraphRenderer';
import './sass/flamegraph.scss';

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
  // Although 'flamegraph' is not a valid HTML element
  // It's used to scope css without affecting specificity
  // For more info see flamegraph.scss
  return (
    <flamegraph is="div">
      {/* eslint-disable-next-line react/jsx-props-no-spreading */}
      <FlameGraphRenderer {...props} {...overrideProps} />
    </flamegraph>
  );
};
