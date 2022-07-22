import React, { useEffect, useState, SetStateAction, Dispatch } from 'react';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';
import classNames from 'classnames';

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
  fetchExplorePage,
  fetchSingleView,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';

import styles from './Explore.module.scss';

function Explore() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();
  const [activeTag, setActiveTag] = useState<string | undefined>(undefined);

  const { from, until, groupByTag, singleView } = useAppSelector(
    selectContinuousState
  );
  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));
  const appName = queryToAppName(query);

  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));
      setActiveTag(undefined);
    }
  }, [query]);

  useEffect(() => {
    if (from && until && query) {
      const fetchData = dispatch(fetchExplorePage(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTag]);

  const handleGroupedByTagChange = (value: string) => {
    dispatch(actions.setGroupByTag(value));
    setActiveTag(undefined);
  };

  const getGroupsData = (): { groups: any; legend: string[] } => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading':
        if (!singleView.groups) {
          return {
            groups: [],
            legend: [],
          };
        }

        const groups = Object.entries(singleView.groups).filter(
          ([key]) => key !== '*'
        );

        if (groups.length > 0) {
          return {
            groups,
            legend: Object.keys(singleView.groups).filter((key) => key !== '*'),
          };
        }

        return {
          groups: [['*', singleView.timeline]],
          legend: [],
        };

      default:
        return {
          groups: [],
          legend: [],
        };
    }
  };

  // group type
  const { groups, legend } = getGroupsData();

  return (
    <div className={styles.explorePage}>
      <Toolbar hideTagsBar />
      <Box>
        <ExploreHeader
          appName={appName}
          tags={tags}
          selectedTag={groupByTag}
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
            setActiveTag={setActiveTag}
            activeTag={activeTag}
          />
        )}
      </Box>
      <Box>
        {activeTag && (
          <FlamegraphRenderer
            showCredit={false}
            profile={singleView.profile}
            colorMode={colorMode}
          />
        )}
      </Box>
    </div>
  );
}

function Table({
  appName,
  groups,
  activeTag,
  setActiveTag,
}: {
  appName: string;
  groups: any[];
  activeTag: string | undefined;
  setActiveTag: Dispatch<SetStateAction<string | undefined>>;
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
              className={tagName === activeTag ? styles.activeTagRow : ''}
              onClick={() => setActiveTag(tagName)}
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

export default Explore;
