import React, { useEffect, useState } from 'react';
import { NavLink, useLocation, Redirect } from 'react-router-dom';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';
import Color from 'color';
import OutsideClickHandler from 'react-outside-click-handler';

import type { Profile } from '@pyroscope/models/src';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import TimelineChartWrapper, {
  TimelineGroupData,
} from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { FlamegraphRenderer, DefaultPalette } from '@pyroscope/flamegraph/src';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import ViewTagsSelectLinkModal from '@webapp/pages/tagExplorer/components/ViewTagsSelectLinkModal';
import useColorMode from '@webapp/hooks/colorMode.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { appendLabelToQuery } from '@webapp/util/query';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
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
  ALL_TAGS,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';
import { calculateMean, calculateStdDeviation } from './math';
import { PAGES } from './constants';

import styles from './TagExplorerView.module.scss';

const getTimelineColor = (index: number, palette: Color[]): Color =>
  Color(palette[index % (palette.length - 1)]);

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

  const { groupByTag, groupByTagValue, type } = tagExplorerView;

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
            acc.push({
              tagName,
              data,
              color: getTimelineColor(index, DefaultPalette.colors),
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

  const exportFlamegraphDotComFn = useExportToFlamegraphDotCom(
    activeTagProfile,
    groupByTag,
    groupByTagValue
  );
  // when there's no groupByTag value backend returns groups with single "*" group,
  // which is "application without any tag" group. when backend returns multiple groups,
  // "*" group samples array is filled with zeros (not longer valid application data).
  // removing "*" group from table data helps to show only relevant data
  const filteredGroupsData =
    groupsData.length === 1
      ? [{ ...groupsData[0], tagName: appName.unwrapOr('') }]
      : groupsData.filter((a) => a.tagName !== '*');

  // filteredGroupsData has single "application without tags" group for initial view
  // its not "real" group so we filter it
  const whereDropdownItems = filteredGroupsData.reduce((acc, group) => {
    if (group.tagName === appName.unwrapOr('')) {
      return acc;
    }

    acc.push(group.tagName);
    return acc;
  }, [] as string[]);

  return (
    <div className={styles.tagExplorerView} data-testid="tag-explorer-view">
      <Toolbar hideTagsBar />
      <Box>
        <ExploreHeader
          appName={appName}
          tags={tags}
          whereDropdownItems={whereDropdownItems}
          selectedTag={tagExplorerView.groupByTag}
          selectedTagValue={tagExplorerView.groupByTagValue}
          handleGroupByTagChange={handleGroupedByTagChange}
          handleGroupByTagValueChange={handleGroupByTagValueChange}
        />
        <div className={styles.timelineWrapper}>
          {type === 'loading' ? (
            <LoadingSpinner />
          ) : (
            <TimelineChartWrapper
              selectionType="double"
              mode="multiple"
              timezone={offset === 0 ? 'utc' : 'browser'}
              data-testid="timeline-explore-page"
              id="timeline-chart-explore-page"
              timelineGroups={filteredGroupsData}
              // to not "dim" timelines when "All" option is selected
              activeGroup={groupByTagValue !== ALL_TAGS ? groupByTagValue : ''}
              showTagsLegend={filteredGroupsData.length > 1}
              handleGroupByTagValueChange={handleGroupByTagValueChange}
              onSelect={(from, until) =>
                dispatch(setDateRange({ from, until }))
              }
              height="125px"
              format="lines"
            />
          )}
        </div>
      </Box>
      <Box>
        <Table
          appName={appName.unwrapOr('')}
          whereDropdownItems={whereDropdownItems}
          groupByTag={groupByTag}
          groupByTagValue={groupByTagValue}
          groupsData={filteredGroupsData}
          handleGroupByTagValueChange={handleGroupByTagValueChange}
          isLoading={type === 'loading'}
        />
      </Box>
      <Box>
        <div className={styles.flamegraphWrapper}>
          {type === 'loading' ? (
            <LoadingSpinner />
          ) : (
            <FlamegraphRenderer
              showCredit={false}
              profile={activeTagProfile}
              colorMode={colorMode}
              ExportData={
                activeTagProfile && (
                  <ExportData
                    flamebearer={activeTagProfile}
                    exportPNG
                    exportJSON
                    exportPprof
                    exportHTML
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportFlamegraphDotComFn}
                  />
                )
              }
            />
          )}
        </div>
      </Box>
    </div>
  );
}

const defaultLinkTagsSelectModalData = {
  baselineTag: '',
  comparisonTag: '',
  linkName: '',
  isModalOpen: false,
  shouldRedirect: false,
};

function Table({
  appName,
  whereDropdownItems,
  groupByTag,
  groupByTagValue,
  groupsData,
  isLoading,
  handleGroupByTagValueChange,
}: {
  appName: string;
  whereDropdownItems: string[];
  groupByTag: string;
  groupByTagValue: string | undefined;
  groupsData: TimelineGroupData[];
  isLoading: boolean;
  handleGroupByTagValueChange: (groupedByTagValue: string) => void;
}) {
  const [linkTagsSelectModalData, setLinkTagsSelectModalData] = useState(
    defaultLinkTagsSelectModalData
  );
  const handleOutsideModalClick = () => {
    setLinkTagsSelectModalData(defaultLinkTagsSelectModalData);
  };

  const { search } = useLocation();
  const isTagSelected = (tag: string) => tag === groupByTagValue;

  const handleTableRowClick = (value: string) => {
    if (value !== groupByTagValue) {
      handleGroupByTagValueChange(value);
    } else {
      handleGroupByTagValueChange(ALL_TAGS);
    }
  };

  const handleLinkModalOpen = (linkName: 'Comparison' | 'Diff') => {
    setLinkTagsSelectModalData((currState) => ({
      ...currState,
      isModalOpen: true,
      linkName,
    }));
  };

  if (linkTagsSelectModalData.shouldRedirect) {
    return (
      <Redirect
        to={
          (linkTagsSelectModalData.linkName === 'Diff'
            ? PAGES.COMPARISON_DIFF_VIEW
            : PAGES.COMPARISON_VIEW) + search
        }
      />
    );
  }

  const getSingleViewSearch = () => {
    if (!groupByTagValue) return search;

    const searchParams = new URLSearchParams(search);
    searchParams.delete('query');
    searchParams.set(
      'query',
      appendLabelToQuery(`${appName}{}`, groupByTag, groupByTagValue)
    );
    return `?${searchParams.toString()}`;
  };

  return (
    <>
      <div className={styles.tableDescription} data-testid="explore-table">
        <span className={styles.title}>{appName} Descriptive Statistics</span>
        <div className={styles.buttons}>
          <NavLink
            to={{
              pathname: PAGES.CONTINOUS_SINGLE_VIEW,
              search: getSingleViewSearch(),
            }}
            exact
          >
            Single
          </NavLink>
          <button
            className={styles.buttonName}
            onClick={() => handleLinkModalOpen('Comparison')}
          >
            Comparison
          </button>
          <button
            className={styles.buttonName}
            onClick={() => handleLinkModalOpen('Diff')}
          >
            Diff
          </button>
          {linkTagsSelectModalData.isModalOpen && (
            <OutsideClickHandler onOutsideClick={handleOutsideModalClick}>
              <ViewTagsSelectLinkModal
                whereDropdownItems={whereDropdownItems}
                groupByTag={groupByTag}
                appName={appName}
                /* eslint-disable-next-line react/jsx-props-no-spreading */
                {...linkTagsSelectModalData}
                setLinkTagsSelectModalData={setLinkTagsSelectModalData}
              />
            </OutsideClickHandler>
          )}
        </div>
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
          {isLoading ? (
            <tr>
              <td colSpan={6}>
                <LoadingSpinner />
              </td>
            </tr>
          ) : (
            groupsData.map(({ tagName, color, data }) => {
              const mean = calculateMean(data.samples);

              return (
                <tr
                  className={isTagSelected(tagName) ? styles.activeTagRow : ''}
                  onClick={
                    // prevent clicking on single "application without tags" group row
                    tagName !== appName
                      ? () => handleTableRowClick(tagName)
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
                  <td>
                    {calculateStdDeviation(data.samples, mean).toFixed(2)}
                  </td>
                  <td>{Math.min(...data.samples)}</td>
                  <td>{Math.max(...data.samples)}</td>
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </>
  );
}

function ExploreHeader({
  appName,
  whereDropdownItems,
  tags,
  selectedTag,
  selectedTagValue,
  handleGroupByTagChange,
  handleGroupByTagValueChange,
}: {
  appName: Maybe<string>;
  whereDropdownItems: string[];
  tags: TagsState;
  selectedTag: string;
  selectedTagValue: string;
  handleGroupByTagChange: (value: string) => void;
  handleGroupByTagValueChange: (value: string) => void;
}) {
  const tagKeys = Object.keys(tags.tags);
  const groupByDropdownItems =
    tagKeys.length > 0 ? tagKeys : ['No tags available'];

  const handleGroupByClick = (e: ClickEvent) => {
    handleGroupByTagChange(e.value);
  };

  const handleGroupByValueClick = (e: ClickEvent) => {
    handleGroupByTagValueChange(e.value);
  };

  return (
    <div className={styles.header} data-testid="explore-header">
      <span className={styles.title}>{appName.unwrapOr('')}</span>
      <div className={styles.query}>
        <span className={styles.selectName}>grouped by</span>
        <Dropdown
          label="select tag"
          value={selectedTag ? `tag: ${selectedTag}` : 'select tag'}
          onItemClick={tagKeys.length > 0 ? handleGroupByClick : undefined}
          menuButtonClassName={
            selectedTag === '' ? styles.notSelectedTagDropdown : undefined
          }
        >
          {groupByDropdownItems.map((tagName) => (
            <MenuItem key={tagName} value={tagName}>
              {tagName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
      <div className={styles.query}>
        <span className={styles.selectName}>where</span>
        <Dropdown
          label="select where"
          value={`${selectedTag ? `${selectedTag} = ` : selectedTag} ${
            selectedTagValue || ALL_TAGS
          }`}
          onItemClick={handleGroupByValueClick}
        >
          {/* always show "All" option */}
          {[ALL_TAGS, ...whereDropdownItems].map((tagGroupName) => (
            <MenuItem key={tagGroupName} value={tagGroupName}>
              {tagGroupName}
            </MenuItem>
          ))}
        </Dropdown>
      </div>
    </div>
  );
}

export default TagExplorerView;
