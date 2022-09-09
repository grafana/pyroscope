/* eslint-disable max-classes-per-file */
//import * as flot from 'flot';
import 'flot';

// Got from https://github.com/rodrigowirth/react-flot/blob/master/src/ReactFlot.d.ts
declare module 'react-flot' {
  interface ReactFlotProps {
    id: string;
    data: ShamefulAny[];
    options: object;
    height?: string;
    width?: string;
  }

  class ReactFlot<CustomProps> extends React.Component<
    ReactFlotProps & CustomProps,
    ShamefulAny
  > {
    componentDidMount(): void;

    // componentWillReceiveProps(nextProps: any): void;

    draw(event?: ShamefulAny, data?: ShamefulAny): void;

    render(): ShamefulAny;
  }
  export = ReactFlot;
}

// From https://github.com/chantastic/react-svg-spinner/blob/master/index.d.ts
declare module 'react-svg-spinner' {
  import React from 'react';

  type SpinnerProps = {
    size?: string;
    width?: string;
    height?: string;
    color?: string;
    thickness?: number;
    gap?: number;
    speed?: 'fast' | 'slow' | 'default';
  };

  // eslint-disable-next-line react/prefer-stateless-function
  class Spinner extends React.Component<SpinnerProps, ShamefulAny> {}

  export default Spinner;
}

// @types/flot only exposes plotOptions
// but flot in fact exposes more parameters to us
// https://github.com/flot/flot/blob/370cf6ee85de0e0fcae5bf084e0986cda343e75b/source/jquery.flot.js#L361
type plotInitPluginParams = jquery.flot.plot & jquery.flot.plotOptions;
declare global {
  declare namespace jquery.flot {
    interface plugin {
      init(plot: plotInitPluginParams): void;
    }
  }
}
