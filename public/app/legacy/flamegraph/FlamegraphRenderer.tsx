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

// Module augmentation so that typescript sees our 'custom' element
declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'pyro-flamegraph': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      >;
    }
  }
}

// TODO: type props
export const FlamegraphRenderer = (
  props: FlamegraphRendererProps & { colorMode?: 'light' | 'dark' }
) => {
  // Although 'flamegraph' is not a valid HTML element
  // It's used to scope css without affecting specificity
  // For more info see flamegraph.scss
  return (
    <pyro-flamegraph
      is="span"
      data-flamegraph-color-mode={props.colorMode || 'dark'}
    >
      {/* eslint-disable-next-line react/jsx-props-no-spreading */}
      <FlameGraphRenderer {...props} {...overrideProps} />
    </pyro-flamegraph>
  );
};
