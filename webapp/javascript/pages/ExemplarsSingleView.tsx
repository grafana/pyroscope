import React, { useEffect, useState } from 'react';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import clsx from 'clsx';

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
import { LoadingOverlay } from '@webapp/ui/LoadingOverlay';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import StatusMessage from '@webapp/ui/StatusMessage';
import { Tooltip } from '@webapp/ui/Tooltip';
import { TooltipInfoIcon } from '@webapp/ui/TooltipInfoIcon';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/components/Heatmap';
import ExportData from '@webapp/components/ExportData';
import ChartTitle from '@webapp/components/ChartTitle';
import ChartTitleStyles from '@webapp/components/ChartTitle.module.scss';
import { DEFAULT_HEATMAP_PARAMS } from '@webapp/components/Heatmap/constants';
import {
  FlamegraphRenderer,
  FlamegraphRendererProps,
} from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import type { Profile } from '@pyroscope/models/src';
import { diffTwoProfiles } from '@pyroscope/flamegraph/src/convert/diffTwoProfiles';
import { subtract } from '@pyroscope/flamegraph/src/convert/subtract';
import { formatTitle } from './formatTitle';
import { isLoadingOrReloading, LoadingType } from './loading';
import heatmapSelectionPreviewGif from './heatmapSelectionPreview.gif';
import { HeatmapSelectionIcon, HeatmapNoSelectionIcon } from './HeatmapIcons';

import styles from './ExemplarsSingleView.module.scss';

function ExemplarsSingleView() {
  const [tabIndex, setTabIndex] = useState(0);
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

  const differenceProfile =
    exemplarsSingleView.profile &&
    exemplarsSingleView.selectionProfile &&
    subtract(exemplarsSingleView.profile, exemplarsSingleView.selectionProfile);

  return (
    <div>
      <PageTitle title={formatTitle('Tracing single', query)} />
      <div className="main-wrapper">
        <Toolbar />
        <Box>
          <p className={styles.heatmapTitle}>Heatmap</p>
          {heatmap}
        </Box>
        {!exemplarsSingleView.selectionProfile && exemplarsSingleView.heatmap && (
          <Box>
            <div className={styles.heatmapSelectionGuide}>
              <StatusMessage
                type="info"
                message="Select an area in the heatmap to get started"
              />
              <img
                className={styles.gif}
                src={heatmapSelectionPreviewGif}
                alt="heatmap-selection-gif"
              />
            </div>
          </Box>
        )}
        {exemplarsSingleView.heatmap &&
        exemplarsSingleView.selectionProfile &&
        differenceProfile ? (
          <Tabs
            selectedIndex={tabIndex}
            onSelect={(index) => setTabIndex(index)}
          >
            <TabList>
              <Tab>Single</Tab>
              <Tab>Comparison</Tab>
              <Tab>Diff</Tab>
            </TabList>
            <TabPanel>
              <SingleTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
            <TabPanel>
              <ComparisonTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                differenceProfile={differenceProfile}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
            <TabPanel>
              <DiffTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                differenceProfile={differenceProfile}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
          </Tabs>
        ) : null}
      </div>
    </div>
  );
}

export default ExemplarsSingleView;

interface TabProps {
  colorMode: FlamegraphRendererProps['colorMode'];
  type: LoadingType;
  selectionProfile: Profile;
}

function SingleTab({ colorMode, type, selectionProfile }: TabProps) {
  return (
    <Box>
      <LoadingOverlay
        active={isLoadingOrReloading([type])}
        spinnerPosition="baseline"
      >
        <FlamegraphRenderer
          showCredit={false}
          profile={selectionProfile}
          colorMode={colorMode}
          ExportData={
            <ExportData
              flamebearer={selectionProfile}
              exportPNG
              exportJSON
              exportPprof
              exportHTML
            />
          }
        />
      </LoadingOverlay>
    </Box>
  );
}

function ComparisonTab({
  colorMode,
  type,
  differenceProfile,
  selectionProfile,
}: TabProps & { differenceProfile: Profile }) {
  return (
    <div className={styles.comparisonTab}>
      <Box className={styles.comparisonTabHalf}>
        <LoadingOverlay
          active={isLoadingOrReloading([type])}
          spinnerPosition="baseline"
        >
          <ChartTitle
            titleKey="selection_included"
            icon={<HeatmapSelectionIcon />}
            postfix={
              <Tooltip
                placement="top"
                title={
                  <div className={styles.titleInfoTooltip}>
                    Represents the aggregated result of all profiles{' '}
                    <b>included within</b> the orange "selected area"
                  </div>
                }
              >
                <TooltipInfoIcon />
              </Tooltip>
            }
          />
          <FlamegraphRenderer
            showCredit={false}
            profile={selectionProfile}
            colorMode={colorMode}
            panesOrientation="vertical"
            ExportData={
              <ExportData
                flamebearer={selectionProfile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
              />
            }
          />
        </LoadingOverlay>
      </Box>
      <Box className={styles.comparisonTabHalf}>
        <LoadingOverlay
          active={isLoadingOrReloading([type])}
          spinnerPosition="baseline"
        >
          <ChartTitle
            titleKey="selection_excluded"
            icon={<HeatmapNoSelectionIcon />}
            postfix={
              <Tooltip
                placement="top"
                title={
                  <div className={styles.titleInfoTooltip}>
                    Represents the aggregated result of all profiles{' '}
                    <b>excluding</b> the orange "selected area"
                  </div>
                }
              >
                <TooltipInfoIcon />
              </Tooltip>
            }
          />
          <FlamegraphRenderer
            showCredit={false}
            profile={differenceProfile}
            colorMode={colorMode}
            panesOrientation="vertical"
            ExportData={
              <ExportData
                flamebearer={differenceProfile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
              />
            }
          />
        </LoadingOverlay>
      </Box>
    </div>
  );
}

function DiffTab({
  colorMode,
  type,
  differenceProfile,
  selectionProfile,
}: TabProps & { differenceProfile: Profile }) {
  const subtractedCopy = JSON.parse(JSON.stringify(differenceProfile));
  const selectionCopy = JSON.parse(JSON.stringify(selectionProfile));
  const diffProfile = diffTwoProfiles(subtractedCopy, selectionCopy);

  return (
    <Box>
      <LoadingOverlay
        active={isLoadingOrReloading([type])}
        spinnerPosition="baseline"
      >
        <ChartTitle
          postfix={
            <Tooltip
              placement="top"
              title={
                <div className={styles.titleInfoTooltip}>
                  Represents the diff between an aggregated flamegraph
                  representing the selected area and an aggregated flamegraph
                  excluding the selected area
                </div>
              }
            >
              <TooltipInfoIcon />
            </Tooltip>
          }
        >
          <span
            className={clsx(
              ChartTitleStyles.colorOrIcon,
              ChartTitleStyles.icon
            )}
          >
            <HeatmapSelectionIcon />
          </span>
          Selection-included vs
          <span
            className={clsx(
              ChartTitleStyles.colorOrIcon,
              ChartTitleStyles.icon
            )}
          >
            <HeatmapNoSelectionIcon />
          </span>
          Selection-excluded Diff Flamegraph
        </ChartTitle>
        <FlamegraphRenderer
          showCredit={false}
          profile={diffProfile}
          colorMode={colorMode}
          ExportData={
            <ExportData
              flamebearer={diffProfile}
              exportPNG
              exportJSON
              exportPprof
              exportHTML
            />
          }
        />
      </LoadingOverlay>
    </Box>
  );
}
