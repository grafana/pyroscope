import React, { useEffect } from 'react';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';

import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
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

  const { groupByTag, groupByTagValue, activeTagProfile, timeline } =
    exploreView;

  useEffect(() => {
    if (from && until && query && groupByTagValue) {
      const fetchData = dispatch(fetchExploreViewProfile(null));
      return () => fetchData.abort('cancel');
    }
  }, [from, until, query, groupByTagValue]);

  useEffect(() => {
    if (from && until && query) {
      const fetchData = dispatch(fetchExploreView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTag]);

  const getGroupsData = (): { groups: any; legend: string[] } => {
    switch (exploreView.type) {
      case 'loaded':
      case 'reloading':
        if (!exploreView.groups) {
          return {
            groups: [],
            legend: [],
          };
        }

        const groups = Object.entries(exploreView.groups).filter(
          ([key]) => key !== '*'
        );

        if (groups.length > 0) {
          return {
            groups,
            legend: Object.keys(exploreView.groups).filter(
              (key) => key !== '*'
            ),
          };
        }

        return {
          // default value ? timeline dependency is wrong ?
          groups: [['<app with no tags data>', exploreView.timeline]],
          legend: [],
        };

      default:
        return {
          groups: [],
          legend: [],
        };
    }
  };

  // legend is timelines color + tag value pair
  const { groups, legend } = getGroupsData();

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
          // add ability to display more then 2 timelines
          timelineA={{
            data: groups.length ? groups[0][1] : undefined,
          }}
          timelineB={{
            data: groups.length && groups[1] ? groups[1][1] : undefined,
          }}
          onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
          height="125px"
          format="lines"
        />
      </Box>
      <Box>
        {appName.isJust && (
          <Table
            appName={appName.value}
            groups={groups}
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
  groups,
  groupByTagValue,
  handleGroupByTagValueChange,
}: {
  appName: string;
  groups: any[];
  groupByTagValue: string | undefined;
  handleGroupByTagValueChange: (groupedByTagValue: string) => void;
}) {
  return (
    <>
      <div className={styles.tableDescription}>
        <span className={styles.title}>{appName} Descriptive Statistics</span>
        <div className={styles.buttons}>
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
          {groups.map(([tagName]) => (
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
