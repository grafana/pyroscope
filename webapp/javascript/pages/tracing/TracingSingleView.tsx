import React from 'react';

import useColorMode from '@webapp/hooks/colorMode.hook';
import { useAppSelector } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/pages/Heatmap/Heatmap';
import ExportData from '@webapp/components/ExportData';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { formatTitle } from '../formatTitle';

import styles from './TracingSingleView.module.scss';

function TracingSingleView() {
  const { colorMode } = useColorMode();
  const { query } = useAppSelector(selectQueries);
  const { heatmapSingleView } = useAppSelector((state) => state.tracing);

  const flamegraphRenderer = (() => {
    switch (heatmapSingleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <FlamegraphRenderer
            showCredit={false}
            profile={heatmapSingleView.profile}
            colorMode={colorMode}
            onlyDisplay="flamegraph"
            ExportData={
              <ExportData
                flamebearer={heatmapSingleView.profile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
              />
            }
          />
        );
      }

      default: {
        return (
          <div className={styles.spinnerWrapper}>
            <LoadingSpinner />
          </div>
        );
      }
    }
  })();

  return (
    <div>
      <PageTitle title={formatTitle('Tracing single', query)} />
      <div className="main-wrapper">
        <Toolbar />
        <Box>
          <p className={styles.heatmapTitle}>Heatmap</p>
          <Heatmap />
        </Box>
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default TracingSingleView;
