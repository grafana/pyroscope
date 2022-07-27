import React, { useEffect } from 'react';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';
import Color from 'color';

import type { Profile } from '@pyroscope/models/src';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import TimelineChartWrapper, {
  TimelineGroupData,
} from '@webapp/components/TimelineChartWrapper';
import { FlamegraphRenderer, DefaultPalette } from '@pyroscope/flamegraph/src';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import useColorMode from '@webapp/hooks/colorMode.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  actions,
  setDateRange,
  fetchTags,
  selectQueries,
  selectContinuousState,
  selectAppTags,
  TagsState,
  fetchExploreView,
  fetchExploreViewProfile,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';

import styles from './ExploreView.module.scss';

function ExploreView() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();

  const { from, until, exploreView, refreshToken } = useAppSelector(
    selectContinuousState
  );
  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));
  const appName = queryToAppName(query);

  // maybe put all effects inside 1 hook ?
  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));
    }
  }, [query]);

  const { groupByTag, groupByTagValue } = exploreView;

  useEffect(() => {
    if (from && until && query && groupByTagValue) {
      const fetchData = dispatch(fetchExploreViewProfile(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTagValue]);

  useEffect(() => {
    if (from && until && query) {
      const fetchData = dispatch(fetchExploreView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTag, refreshToken]);

  const getGroupsData = (): {
    groupsData: TimelineGroupData[];
    activeTagProfile?: Profile;
  } => {
    switch (exploreView.type) {
      case 'loaded':
      case 'reloading':
        if (!exploreView.groups) {
          return {
            groupsData: [],
            activeTagProfile: undefined,
          };
        }

        const groups = Object.entries(exploreView.groups).reduce(
          (acc, [tagName, data], index) => {
            if (tagName === '*' || index === 15) return acc;

            acc.push({
              tagName,
              data,
              color: Color(DefaultPalette.colors[index]),
            });

            return acc;
          },
          [] as TimelineGroupData[]
        );

        if (groups.length > 0) {
          return {
            groupsData: groups,
            activeTagProfile: exploreView?.activeTagProfile,
          };
        }

        return {
          groupsData: [{ tagName: '*', data: exploreView.timeline }],
          activeTagProfile: undefined,
        };

      default:
        return {
          groupsData: [],
          activeTagProfile: undefined,
        };
    }
  };

  const { groupsData, activeTagProfile } = getGroupsData();

  const handleGroupByTagValueChange = (groupByTagValue: string) => {
    dispatch(actions.setExploreViewGroupByTagValue(groupByTagValue));
  };

  const handleGroupedByTagChange = (value: string) => {
    dispatch(actions.setExploreViewGroupByTag(value));
  };

  return (
    <div className={styles.exploreView}>
      <Toolbar hideTagsBar />
      <Box>
        <ExploreHeader
          appName={appName}
          tags={tags}
          selectedTag={exploreView.groupByTag}
          handleTagChange={handleGroupedByTagChange}
        />
        <TimelineChartWrapper
          timezone={offset === 0 ? 'utc' : 'browser'}
          data-testid="timeline-explore-page"
          id="timeline-chart-explore-page"
          timelineA={{ data: undefined }}
          timelineB={{ data: undefined }}
          timelineGroups={groupsData}
          onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
          height="125px"
          format="lines"
        />
      </Box>
      <Box>
        {appName.isJust && (
          <Table
            appName={appName.value}
            groupsData={groupsData}
            groupByTagValue={groupByTagValue}
            handleGroupByTagValueChange={handleGroupByTagValueChange}
          />
        )}
      </Box>
      {groupByTagValue && (
        <Box>
          <FlamegraphRenderer
            showCredit={false}
            profile={activeTagProfile}
            colorMode={colorMode}
          />
        </Box>
      )}
    </div>
  );
}

export function calculateMean(arr: number[]) {
  return arr.reduce((acc, b) => acc + b, 0) / arr.length;
}

export function calculateStdDeviation(array: number[], mean: number) {
  const stdDeviation = Math.sqrt(
    array.reduce((acc, b) => {
      const dev = b - mean;

      return acc + dev ** 2;
    }, 0) / array.length
  );

  return stdDeviation;
}

function Table({
  appName,
  groupsData,
  groupByTagValue,
  handleGroupByTagValueChange,
}: {
  appName: string;
  groupsData: TimelineGroupData[];
  groupByTagValue: string | undefined;
  handleGroupByTagValueChange: (groupedByTagValue: string) => void;
}) {
  return (
    <>
      <div className={styles.tableDescription}>
        <span className={styles.title}>{appName} Descriptive Statistics</span>
        <div>
          <button>Export</button>
          <button>Single</button>
          <button>Comparison</button>
          <button>Diff</button>
        </div>
      </div>
      <table className={styles.exploreTable}>
        <thead>
          <tr>
            <th>{groupsData[0]?.tagName === '*' ? 'App' : 'Tag'} name</th>
            <th>Event count</th>
            <th>avg samples</th>
            <th>std deviation samples</th>
            <th>min samples</th>
            <th>max samples</th>
            <th>50% tbd</th>
            <th>75% tbd</th>
            <th>max tbd</th>
            <th>cost tbd</th>
          </tr>
        </thead>
        <tbody>
          {groupsData.map(({ tagName, color, data }) => {
            const mean = calculateMean(data.samples);

            return (
              <tr
                className={
                  tagName === groupByTagValue ? styles.activeTagRow : ''
                }
                onClick={() => handleGroupByTagValueChange(tagName)}
                key={tagName}
              >
                <td>
                  {tagName === '*' ? (
                    appName
                  ) : (
                    <div className={styles.tagName}>
                      <span
                        className={styles.tagColor}
                        style={{ backgroundColor: color?.toString() }}
                      />
                      {tagName}
                    </div>
                  )}
                </td>
                <td>{data.samples.length}</td>
                <td>{mean.toFixed(2)}</td>
                <td>{calculateStdDeviation(data.samples, mean).toFixed(2)}</td>
                <td>{Math.min(...data.samples)}</td>
                <td>{Math.max(...data.samples)}</td>
                <td>50,987 mock</td>
                <td>76,200 mock</td>
                <td>100,000 mock</td>
                <td>$ 250 / hr mock</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </>
  );
}

function ExploreHeader({
  appName,
  tags,
  selectedTag,
  handleTagChange,
}: {
  appName: Maybe<string>;
  tags: TagsState;
  selectedTag: string;
  handleTagChange: (value: string) => void;
}) {
  const tagKeys = Object.keys(tags.tags);

  const handleClick = (e: ClickEvent) => {
    handleTagChange(e.value);
  };

  return (
    <div className={styles.header}>
      <span className={styles.title}>
        {appName.isJust ? appName.value : ''}
      </span>
      <div className={styles.query}>
        <span className={styles.selectName}>grouped by</span>
        <Dropdown
          label="tags"
          value={selectedTag ? `tag: ${selectedTag}` : 'select tag'}
          onItemClick={handleClick}
        >
          {tagKeys.map((tagName) => (
            <MenuItem key={tagName} value={tagName}>
              {tagName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
    </div>
  );
}

export default ExploreView;
