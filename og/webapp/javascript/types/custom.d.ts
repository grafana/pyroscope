/* eslint-disable max-classes-per-file */

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

  export default React.FC<SpinnerProps>();
}
