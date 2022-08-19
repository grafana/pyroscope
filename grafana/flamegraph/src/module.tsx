import React from 'react';
import { ExplorePanelProps, PanelPlugin, PanelProps } from '@grafana/data';
import FlameGraphContainer from './components/FlameGraphContainer';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = (props) => {
  return <FlameGraphContainer data={props.data.series[0]} />;
};

export const FlameExploreGraphPanel: React.FunctionComponent<ExplorePanelProps> = (props) => {
  return <FlameGraphContainer data={props.data[0]} />;
};

export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameExploreGraphPanel, ['profile']);
