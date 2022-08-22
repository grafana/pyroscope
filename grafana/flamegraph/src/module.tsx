import React from 'react';
// @ts-ignore
import { ExplorePanelProps, PanelPlugin, PanelProps } from '@grafana/data';
import FlameGraphContainer from './components/FlameGraphContainer';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = (props) => {
  return <FlameGraphContainer data={props.data.series[0]} />;
};

export const FlameExploreGraphPanel: React.FunctionComponent<ExplorePanelProps> = (props) => {
  return <FlameGraphContainer data={props.data[0]} />;
};

// We use ts-ignore here because setExplorePanel and ExplorePanelProps are part of a draft PR that isn't yet merged.
// We could solve this by linking but that has quite a bit of issues with regard of resolving dependencies downstream
// in grafana/data and also needs some custom modification in grafana repo so for now this seems to be easier as the
// there is not that much to the API.
// @ts-ignore
export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameExploreGraphPanel, ['profile']);
