import React from 'react';
// @ts-ignore
import { ExplorePanelProps, GrafanaTheme2, PanelPlugin, PanelProps } from '@grafana/data';
import FlameGraphContainer from './components/FlameGraphContainer';
import { useStyles2 } from '@grafana/ui';
import { css } from '@emotion/css';

export const FlameGraphPanel: React.FunctionComponent<PanelProps> = (props) => {
  return <FlameGraphContainer data={props.data.series[0]} />;
};

export const FlameExploreGraphPanel: React.FunctionComponent<ExplorePanelProps> = (props) => {
  const styles = useStyles2((theme) => getStyles(theme));

  return (
    <div className={styles.container}>
      <FlameGraphContainer data={props.data[0]} />
    </div>
  )
};

// We use ts-ignore here because setExplorePanel and ExplorePanelProps are part of a draft PR that isn't yet merged.
// We could solve this by linking but that has quite a bit of issues with regard of resolving dependencies downstream
// in grafana/data and also needs some custom modification in grafana repo so for now this seems to be easier as the
// there is not that much to the API.
// @ts-ignore
export const plugin = new PanelPlugin(FlameGraphPanel).setExplorePanel(FlameExploreGraphPanel, ['profile']);

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    background: ${theme.colors.background.primary};
    display: flow-root;
    padding: ${theme.spacing(1)};
    border: 1px solid ${theme.components.panel.borderColor};
    border-radius: ${theme.shape.borderRadius(1)};
  `,
});
