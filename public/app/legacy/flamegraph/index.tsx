// eslint-disable-next-line import/no-extraneous-dependencies
import Box from '@pyroscope/ui/Box';
import Flamegraph from './FlameGraph/FlameGraphComponent/Flamegraph';
import { FlamegraphRenderer } from './FlamegraphRenderer';
import { DefaultPalette } from './FlameGraph/FlameGraphComponent/colorPalette';
import { convertJaegerTraceToProfile } from './convert/convertJaegerTraceToProfile';
import { diffTwoProfiles } from './convert/diffTwoProfiles';

export {
  Flamegraph,
  DefaultPalette,
  FlamegraphRenderer,
  Box,
  convertJaegerTraceToProfile,
  diffTwoProfiles,
};
