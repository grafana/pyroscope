/* eslint-disable import/prefer-default-export */
// eslint-disable-next-line import/no-extraneous-dependencies
import Box from '@pyroscope/ui/Box';
import Flamegraph from './FlameGraph/FlameGraphComponent/Flamegraph';
import { DefaultPalette } from './FlameGraph/FlameGraphComponent/colorPalette';
import { FlamegraphRenderer } from './FlamegraphRenderer';
import { convertJaegerTraceToProfile } from './convert/convertJaegerTraceToProfile';
import { diffTwoProfiles } from './convert/diffTwoProfiles';
import { subtract } from './convert/subtract';

export {
  Flamegraph,
  DefaultPalette,
  FlamegraphRenderer,
  Box,
  convertJaegerTraceToProfile,
  diffTwoProfiles,
  subtract,
};
