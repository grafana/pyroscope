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
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
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

const timelineSeriesColors = [
  'blueviolet',
  'lime',
  'deepskyblue',
  'chocolate',
  'yellowgreen',
  'lightsalmon',
  'rosybrown',
  'maroon',
  'orangered',
  'red',
  'orange',
  'yellow',
  'green',
  'blue',
  'hotpink',
];

function ExploreView() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();

  const { from, until, exploreView } = useAppSelector(selectContinuousState);
  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));
  const appName = queryToAppName(query);

  // maybe put all effects inside 1 hook ?
  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));
      // setActiveTag(undefined);
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
  }, [from, until, query, groupByTag]);

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
              color: Color(timelineSeriesColors[index]),
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
          groupsData: [
            { tagName: '<app with no tags data>', data: exploreView.timeline },
          ],
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
      <Box>
        {groupByTagValue && (
          <FlamegraphRenderer
            showCredit={false}
            profile={activeTagProfile}
            colorMode={colorMode}
          />
        )}
      </Box>
    </div>
  );
}

// remove timeline dep from table
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
      <table>
        <thead>
          <tr>
            <th>Tag name</th>
            <th>10s event count</th>
            <th>avg samples per 10s</th>
            <th>samples std. deviation</th>
            <th>min samples</th>
            <th>25%</th>
            <th>50%</th>
            <th>75%</th>
            <th>max</th>
            <th>cost</th>
          </tr>
        </thead>
        <tbody>
          {groupsData.map(({ tagName }) => (
            <tr
              className={tagName === groupByTagValue ? styles.activeTagRow : ''}
              onClick={() => handleGroupByTagValueChange(tagName)}
              key={tagName}
            >
              {/* mock data */}
              <td>{tagName}</td>
              <td>15,000</td>
              <td>3,276</td>
              <td>1,532</td>
              <td>3,188</td>
              <td>25,333</td>
              <td>50,987</td>
              <td>76,200</td>
              <td>100,000</td>
              <td>$ 250 / hr</td>
            </tr>
          ))}
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
