import React, { useEffect, useState } from 'react';
import clsx from 'clsx';
import { Tabs, Tab, TabPanel } from '@pyroscope/ui/Tabs';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import useTags from '@pyroscope/hooks/tags.hook';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import {
  actions,
  fetchTagValues,
  selectQueries,
  setQuery,
} from '@pyroscope/redux/reducers/continuous';
import {
  fetchExemplarsSingleView,
  fetchSelectionProfile,
} from '@pyroscope/redux/reducers/tracing';
import Box from '@pyroscope/ui/Box';
import NoData from '@pyroscope/ui/NoData';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import LoadingSpinner from '@pyroscope/ui/LoadingSpinner';
import StatusMessage from '@pyroscope/ui/StatusMessage';
import { Tooltip } from '@pyroscope/ui/Tooltip';
import { TooltipInfoIcon } from '@pyroscope/ui/TooltipInfoIcon';
import Toolbar from '@pyroscope/components/Toolbar';
import TagsBar from '@pyroscope/components/TagsBar';
import PageTitle from '@pyroscope/components/PageTitle';
import { Heatmap } from '@pyroscope/components/Heatmap';
import ExportData from '@pyroscope/components/ExportData';
import ChartTitle from '@pyroscope/components/ChartTitle';
/* eslint-disable */
import ChartTitleStyles from '@pyroscope/components/ChartTitle.module.scss';
import { DEFAULT_HEATMAP_PARAMS } from '@pyroscope/components/Heatmap/constants';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import type { Profile } from '@pyroscope/legacy/models';
import { diffTwoProfiles } from '@pyroscope/legacy/flamegraph/convert/diffTwoProfiles';
import { subtract } from '@pyroscope/legacy/flamegraph/convert/subtract';
import { formatTitle } from '../formatTitle';
import { isLoadingOrReloading, LoadingType } from '../loading';
import heatmapSelectionPreviewGif from './heatmapSelectionPreview.gif';
import { HeatmapSelectionIcon, HeatmapNoSelectionIcon } from './HeatmapIcons';

import styles from './ExemplarsSingleView.module.scss';
import { filterNonCPU } from './filterNonCPU';
import { Panel } from '@pyroscope/components/Panel';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';

function ExemplarsSingleView() {
  const [tabIndex, setTabIndex] = useState(0);
  const { colorMode } = useColorMode();
  const { offset } = useTimeZone();
  const tags = useTags().regularTags;

  const { query } = useAppSelector(selectQueries);
  const { from, until } = useAppSelector((state) => state.continuous);
  const { exemplarsSingleView } = useAppSelector((state) => state.tracing);
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
  }, [from, until, query, dispatch]);

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
      <PageContentWrapper>
        <Toolbar
          onSelectedApp={(query) => {
            dispatch(setQuery(query));
          }}
          filterApp={filterNonCPU}
        />
        <TagsBar
          query={query}
          tags={tags}
          onRefresh={() => dispatch(actions.refresh())}
          onSetQuery={(q) => dispatch(actions.setQuery(q))}
          onSelectedLabel={(label, query) => {
            dispatch(fetchTagValues({ query, label }));
          }}
        />

        <Box>
          <p className={styles.heatmapTitle}>Heatmap</p>
          {heatmap}
        </Box>
        {!exemplarsSingleView.selectionProfile &&
          exemplarsSingleView.heatmap && (
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
          <>
            <Tabs value={tabIndex} onChange={(e, value) => setTabIndex(value)}>
              <Tab label="Single" />
              <Tab label="Comparison" />
              <Tab label="Diff" />
            </Tabs>
            <TabPanel value={tabIndex} index={0}>
              <SingleTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
            <TabPanel value={tabIndex} index={1}>
              <ComparisonTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                differenceProfile={differenceProfile}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
            <TabPanel value={tabIndex} index={2}>
              <DiffTab
                colorMode={colorMode}
                type={exemplarsSingleView.type}
                differenceProfile={differenceProfile}
                selectionProfile={exemplarsSingleView.selectionProfile}
              />
            </TabPanel>
          </>
        ) : null}
      </PageContentWrapper>
    </div>
  );
}

export default ExemplarsSingleView;

interface TabProps {
  colorMode: ReturnType<typeof useColorMode>['colorMode'];
  type: LoadingType;
  selectionProfile: Profile;
}

function SingleTab({ colorMode, type, selectionProfile }: TabProps) {
  return (
    <Panel isLoading={isLoadingOrReloading([type])}>
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
    </Panel>
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
      <Panel
        className={styles.comparisonTabHalf}
        isLoading={isLoadingOrReloading([type])}
        title={
          <ChartTitle
            titleKey="selection_included"
            icon={<HeatmapSelectionIcon />}
            postfix={
              <Tooltip
                placement="top"
                title={
                  <div className={styles.titleInfoTooltip}>
                    Represents the aggregated result of all profiles{' '}
                    <b>included within</b> the orange &quot;selected area&quot;
                  </div>
                }
              >
                <TooltipInfoIcon />
              </Tooltip>
            }
          />
        }
      >
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
      </Panel>
      <Panel
        className={styles.comparisonTabHalf}
        isLoading={isLoadingOrReloading([type])}
        title={
          <ChartTitle
            titleKey="selection_excluded"
            icon={<HeatmapNoSelectionIcon />}
            postfix={
              <Tooltip
                placement="top"
                title={
                  <div className={styles.titleInfoTooltip}>
                    Represents the aggregated result of all profiles{' '}
                    <b>excluding</b> the orange &quot;selected area&quot;
                  </div>
                }
              >
                <TooltipInfoIcon />
              </Tooltip>
            }
          />
        }
      >
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
      </Panel>
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
