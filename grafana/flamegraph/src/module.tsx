import React from 'react';
import { PanelPlugin, PanelProps } from '@grafana/data';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = () => {
  return <div>Flame Graph pip pup pip pip (r2d2 noises)</div>;
};

export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameGraphPanel, ['profile']);
