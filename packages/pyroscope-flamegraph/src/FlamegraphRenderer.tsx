import React from 'react';
import { Profile } from '@pyroscope/models';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore: Until we rewrite FlamegraphRenderer in typescript this will do
import FlameGraphRenderer from './FlameGraph/FlameGraphRenderer';

const overrideProps = {
  //  showPyroscopeLogo: !process.env.PYROSCOPE_HIDE_LOGO as any, // this is injected by webpack
  showPyroscopeLogo: false,
};

export type FlamegraphRendererProps = {
  profile: Profile;
} & any;
// TODO: type props
export const FlamegraphRenderer = (props: FlamegraphRendererProps) => {
  // eslint-disable-next-line react/jsx-props-no-spreading
  return <FlameGraphRenderer {...props} {...overrideProps} />;
};
