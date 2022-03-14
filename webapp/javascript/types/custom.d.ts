/* eslint-disable max-classes-per-file */
// https://github.com/Connormiha/jest-css-modules-transform/issues/33
declare module '*.module.css' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.scss' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.sass' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.less' {
  const classes: { [key: string]: string };
  export default classes;
}

declare module '*.module.styl' {
  const classes: { [key: string]: string };
  export default classes;
}

// https://stackoverflow.com/a/45887328
declare module '*.svg' {
  const content: ShamefulAny;
  export default content;
}

// Got from https://github.com/rodrigowirth/react-flot/blob/master/src/ReactFlot.d.ts
declare module 'react-flot' {
  interface ReactFlotProps {
    id: string;
    data: ShamefulAny[];
    options: object;
    height: string;
    width: string;
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
