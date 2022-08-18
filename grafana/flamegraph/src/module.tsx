import React from 'react';
import { PanelPlugin, PanelProps } from '@grafana/data';
import FlameGraphContainer from './components/FlameGraphContainer';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = () => {
  return (
    <FlameGraphContainer/>
  );
};

export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameGraphPanel, ['profile']);
