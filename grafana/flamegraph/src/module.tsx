import React from 'react';
import { PanelPlugin, PanelProps } from '@grafana/data';
import FlameGraph from './components/FlameGraph';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = () => {
  return (
    <FlameGraph />
  );
};

export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameGraphPanel, ['profile']);
