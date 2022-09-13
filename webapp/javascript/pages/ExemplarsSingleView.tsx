import React, { useEffect } from 'react';

import useColorMode from '@webapp/hooks/colorMode.hook';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';
import {
  fetchExemplarsSingleView,
  fetchSelectionProfile,
} from '@webapp/redux/reducers/tracing';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/components/Heatmap';
import ExportData from '@webapp/components/ExportData';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import { DEFAULT_HEATMAP_PARAMS } from '@webapp/components/Heatmap/constants';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { formatTitle } from './formatTitle';

import styles from './ExemplarsSingleView.module.scss';

function ExemplarsSingleView() {
  const { colorMode } = useColorMode();
  const { query } = useAppSelector(selectQueries);
  const { exemplarsSingleView } = useAppSelector((state) => state.tracing);
  const { from, until } = useAppSelector((state) => state.continuous);
  const dispatch = useAppDispatch();

  useEffect(() => {
    if (from && until && query) {
      const fetchData = dispatch(
        fetchExemplarsSingleView({
          query,
          from,
          until,
          ...DEFAULT_HEATMAP_PARAMS,
        })
      );
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query]);

  const handleHeatmapSelection = (
    minValue: number,
    maxValue: number,
    startTime: number,
    endTime: number
  ) => {
    dispatch(
      fetchSelectionProfile({
        from,
        until,
        query,
        heatmapTimeBuckets: DEFAULT_HEATMAP_PARAMS.heatmapTimeBuckets,
        heatmapValueBuckets: DEFAULT_HEATMAP_PARAMS.heatmapValueBuckets,
        selectionMinValue: minValue,
        selectionMaxValue: maxValue,
        selectionStartTime: startTime,
        selectionEndTime: endTime,
      })
    );
  };

  const flamegraphRenderer = (() => {
    switch (exemplarsSingleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <FlamegraphRenderer
            showCredit={false}
            profile={exemplarsSingleView.profile}
            colorMode={colorMode}
            onlyDisplay="flamegraph"
            ExportData={
              <ExportData
                flamebearer={exemplarsSingleView.profile}
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

  const heatmap = (() => {
    switch (exemplarsSingleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <Heatmap
            heatmap={exemplarsSingleView.heatmap}
            onSelection={handleHeatmapSelection}
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
          {heatmap}
        </Box>
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default ExemplarsSingleView;
