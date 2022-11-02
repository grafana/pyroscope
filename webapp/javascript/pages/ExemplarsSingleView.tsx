import React, { useEffect } from 'react';

import useColorMode from '@webapp/hooks/colorMode.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';
import {
  fetchExemplarsSingleView,
  fetchSelectionProfile,
} from '@webapp/redux/reducers/tracing';
import Box from '@webapp/ui/Box';
import NoData from '@webapp/ui/NoData';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/components/Heatmap';
import ExportData from '@webapp/components/ExportData';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import StatusMessage from '@webapp/ui/StatusMessage';
import { DEFAULT_HEATMAP_PARAMS } from '@webapp/components/Heatmap/constants';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { formatTitle } from './formatTitle';
import heatmapSelectionGif from './heatmapSelection.gif';

import styles from './ExemplarsSingleView.module.scss';

function ExemplarsSingleView() {
  const { colorMode } = useColorMode();
  const { offset } = useTimeZone();

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
          shouldFetchProfile: !!exemplarsSingleView.profile,
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
        return exemplarsSingleView.profile ? (
          <Box>
            <FlamegraphRenderer
              showCredit={false}
              profile={exemplarsSingleView.profile}
              colorMode={colorMode}
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
          </Box>
        ) : null;
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
        return exemplarsSingleView.heatmap !== null ? (
          <Heatmap
            heatmap={exemplarsSingleView.heatmap}
            onSelection={handleHeatmapSelection}
            timezone={offset === 0 ? 'utc' : 'browser'}
            sampleRate={exemplarsSingleView.profile?.metadata.sampleRate || 100}
          />
        ) : (
          <NoData />
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
        {!exemplarsSingleView.profile && (
          <Box>
            <div className={styles.heatmapSelectionGuide}>
              <p className={styles.title}>
                Select an area in the heatmap to get started
              </p>
              <img
                className={styles.gif}
                src={heatmapSelectionGif}
                alt="heatmap-selection-gif"
              />
              <StatusMessage
                type="info"
                message="Select an area in the heatmap to get started"
              />
            </div>
          </Box>
        )}
        {exemplarsSingleView.heatmap ? flamegraphRenderer : null}
      </div>
    </div>
  );
}

export default ExemplarsSingleView;
