/* eslint-disable react/react-in-jsx-scope */
/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import FlameGraphRenderer from '../javascript/components/FlameGraph/FlameGraphRenderer';
import Flamegraph from '../javascript/components/FlameGraph/FlameGraphComponent/Flamegraph';
import ExportData from '../javascript/components/ExportData';
import { decodeFlamebearer } from '../javascript/models/flamebearer';

const overrideProps = {
  showPyroscopeLogo: true,
};

// TODO: type props
const FlamegraphRenderer = (props: any) => {
  return <FlameGraphRenderer {...props} {...overrideProps} />;
};

export { FlamegraphRenderer, Flamegraph, ExportData, decodeFlamebearer };
