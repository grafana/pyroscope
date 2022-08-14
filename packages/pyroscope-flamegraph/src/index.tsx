// eslint-disable-next-line import/no-extraneous-dependencies
import Box from '@webapp/ui/Box';
import Flamegraph from './FlameGraph/FlameGraphComponent/Flamegraph';
import { FlamegraphRenderer } from './FlamegraphRenderer';
import {
  DefaultPalette,
  TimelineSeriesPalette,
} from './FlameGraph/FlameGraphComponent/colorPalette';
import { convertJaegerTraceToProfile } from './convert/convertJaegerTraceToProfile';
import { diffTwoProfiles } from './convert/diffTwoProfiles';

export {
  Flamegraph,
  DefaultPalette,
  TimelineSeriesPalette,
  FlamegraphRenderer,
  Box,
  convertJaegerTraceToProfile,
  diffTwoProfiles,
};
