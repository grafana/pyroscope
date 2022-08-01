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
  fetchTagExplorerView,
  fetchTagExplorerViewProfile,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';
import { calculateMean, calculateStdDeviation } from './math';

import styles from './TagExplorerView.module.scss';

function TagExplorerView() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();

  const { from, until, tagExplorerView, refreshToken } = useAppSelector(
    selectContinuousState
  );
  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));
  const appName = queryToAppName(query);

  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));
    }
  }, [query]);

  const { groupByTag, groupByTagValue } = tagExplorerView;

  useEffect(() => {
    if (from && until && query && groupByTagValue) {
      const fetchData = dispatch(fetchTagExplorerViewProfile(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTagValue]);

  useEffect(() => {
    if (from && until && query) {
      const fetchData = dispatch(fetchTagExplorerView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, groupByTag, refreshToken]);

  const getGroupsData = (): {
    groupsData: TimelineGroupData[];
    activeTagProfile?: Profile;
  } => {
    switch (tagExplorerView.type) {
      case 'loaded':
      case 'reloading':
        const groups = Object.entries(tagExplorerView.groups).reduce(
          (acc, [tagName, data], index) => {
            if (index === 15) return acc;

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
            activeTagProfile: tagExplorerView?.activeTagProfile,
          };
        }

        return {
          groupsData: [],
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
    dispatch(actions.setTagExplorerViewGroupByTagValue(groupByTagValue));
  };

  const handleGroupedByTagChange = (value: string) => {
    dispatch(actions.setTagExplorerViewGroupByTag(value));
  };

  // when there's no groupByTag value backend returns groups with single "*" group,
  // which is "application without any tag" group. when backend returns multiple groups,
  // "*" group samples array is filled with zeros (not longer valid application data).
  // removing "*" group from table data helps to show only relevant data
  const filteredGroupsData =
    groupsData.length === 1
      ? [{ ...groupsData[0], tagName: appName.unwrapOr('') }]
      : groupsData.filter((a) => a.tagName !== '*');

  return (
    <div className={styles.tagExplorerView}>
      <Toolbar hideTagsBar />
      <Box>
        <ExploreHeader
          appName={appName}
          tags={tags}
          selectedTag={tagExplorerView.groupByTag}
          handleTagChange={handleGroupedByTagChange}
        />
        <TimelineChartWrapper
          mode="multiple"
          timezone={offset === 0 ? 'utc' : 'browser'}
          data-testid="timeline-explore-page"
          id="timeline-chart-explore-page"
          timelineGroups={filteredGroupsData}
          activeGroup={groupByTagValue}
          showTagsLegend={filteredGroupsData.length > 1}
          onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
          height="125px"
          format="lines"
        />
      </Box>
      <Box>
        {appName.isJust && (
          <Table
            appName={appName.value}
            groupByTag={groupByTag}
            groupByTagValue={groupByTagValue}
            groupsData={filteredGroupsData}
            handleGroupByTagValueChange={handleGroupByTagValueChange}
          />
        )}
      </Box>
      {(groupByTag === '' || groupByTagValue) && (
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

function Table({
  appName,
  groupByTag,
  groupByTagValue,
  groupsData,
  handleGroupByTagValueChange,
}: {
  appName: string;
  groupByTag: string;
  groupByTagValue: string | undefined;
  groupsData: TimelineGroupData[];
  handleGroupByTagValueChange: (groupedByTagValue: string) => void;
}) {
  const isTagSelected = (tag: string) => tag === groupByTagValue;

  return (
    <>
      <div className={styles.tableDescription}>
        <span className={styles.title}>{appName} Descriptive Statistics</span>
      </div>
      <table className={styles.tagExplorerTable}>
        <thead>
          <tr>
            {/* when groupByTag is not selected table represents single "application without tags" group */}
            <th>{groupByTag === '' ? 'App' : 'Tag'} name</th>
            <th>Event count</th>
            <th>avg samples</th>
            <th>std deviation samples</th>
            <th>min samples</th>
            <th>max samples</th>
          </tr>
        </thead>
        <tbody>
          {groupsData.map(({ tagName, color, data }) => {
            const mean = calculateMean(data.samples);

            return (
              <tr
                className={isTagSelected(tagName) ? styles.activeTagRow : ''}
                onClick={
                  // prevent clicking on single "application without tags" group row
                  tagName !== appName
                    ? () => handleGroupByTagValueChange(tagName)
                    : undefined
                }
                key={tagName}
              >
                <td>
                  <div className={styles.tagName}>
                    <span
                      className={styles.tagColor}
                      style={{ backgroundColor: color?.toString() }}
                    />
                    {tagName}
                  </div>
                </td>
                <td>{data.samples.length}</td>
                <td>{mean.toFixed(2)}</td>
                <td>{calculateStdDeviation(data.samples, mean).toFixed(2)}</td>
                <td>{Math.min(...data.samples)}</td>
                <td>{Math.max(...data.samples)}</td>
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
  const dropdownItems = tagKeys.length > 0 ? tagKeys : ['No tags available'];

  const handleClick = (e: ClickEvent) => {
    handleTagChange(e.value);
  };

  return (
    <div className={styles.header}>
      <span className={styles.title}>{appName.unwrapOr('')}</span>
      <div className={styles.query}>
        <span className={styles.selectName}>grouped by</span>
        <Dropdown
          label="select tag"
          value={selectedTag ? `tag: ${selectedTag}` : 'select tag'}
          onItemClick={tagKeys.length > 0 ? handleClick : undefined}
          menuButtonClassName={
            selectedTag === '' ? styles.notSelectedTagDropdown : undefined
          }
        >
          {dropdownItems.map((tagName) => (
            <MenuItem key={tagName} value={tagName}>
              {tagName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
    </div>
  );
}

export default TagExplorerView;
