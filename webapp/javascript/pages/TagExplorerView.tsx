import React, { useEffect } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';
import Color from 'color';
import PieChart from '@webapp/pages/tagExplorer/components/PieChart';
import type { Profile } from '@pyroscope/models/src';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import TimelineChartWrapper, {
  TimelineGroupData,
} from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import TagsSelector from '@webapp/pages/tagExplorer/components/TagsSelector';
import TableUI, { BodyRow } from '@webapp/ui/Table';
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
import PageTitle from '@webapp/components/PageTitle';
import ExploreTooltip from '@webapp/components/TimelineChart/ExploreTooltip';
import { calculateMean, calculateStdDeviation } from './math';
import { PAGES } from './constants';

// eslint-disable-next-line css-modules/no-unused-class
import styles from './TagExplorerView.module.scss';
import { formatTitle } from './formatTitle';

const TIMELINE_SERIES_COLORS = [
  Color.rgb(115, 191, 105),
  Color.rgb(244, 213, 152),
  Color.rgb(174, 162, 224),
  Color.rgb(112, 219, 237),
  Color.rgb(224, 117, 45),
  Color.rgb(191, 27, 0),
  Color.rgb(229, 168, 226),
  Color.rgb(183, 219, 171),
  Color.rgb(229, 172, 14),
  Color.rgb(10, 80, 161),
  Color.rgb(97, 77, 147),
  Color.rgb(100, 176, 200),
  Color.rgb(150, 45, 130),
  Color.rgb(214, 131, 206),
  Color.rgb(63, 104, 51),
  Color.rgb(150, 115, 2),
  Color.rgb(5, 43, 81),
  Color.rgb(63, 43, 91),
  Color.rgb(153, 68, 10),
  Color.rgb(88, 20, 12),
  Color.rgb(224, 249, 215),
  Color.rgb(252, 234, 202),
  Color.rgb(207, 250, 255),
  Color.rgb(249, 226, 210),
  Color.rgb(252, 226, 222),
  Color.rgb(186, 223, 244),
  Color.rgb(249, 217, 249),
  Color.rgb(222, 218, 247),
];

const TIMELINE_WRAPPER_ID = 'explore_timeline_wrapper';

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
              color: getTimelineColor(index, TIMELINE_SERIES_COLORS),
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

  const handleGroupByTagValueChange = (v: string) => {
    dispatch(actions.setTagExplorerViewGroupByTagValue(v));
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
    <>
      <PageTitle title={formatTitle('Tag Explorer View', query)} />
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
          <div id={TIMELINE_WRAPPER_ID} className={styles.timelineWrapper}>
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
                activeGroup={
                  groupByTagValue !== ALL_TAGS ? groupByTagValue : ''
                }
                showTagsLegend={filteredGroupsData.length > 1}
                handleGroupByTagValueChange={handleGroupByTagValueChange}
                onSelect={(from, until) =>
                  dispatch(setDateRange({ from, until }))
                }
                height="125px"
                format="lines"
                onHoverDisplayTooltip={(data) => (
                  <ExploreTooltip
                    values={data.values}
                    timeLabel={data.timeLabel}
                    profile={activeTagProfile}
                  />
                )}
              />
            )}
          </div>
        </Box>
        <Box>
          <div className={styles.statiscticsBox}>
            <Table
              appName={appName.unwrapOr('')}
              whereDropdownItems={whereDropdownItems}
              groupByTag={groupByTag}
              groupByTagValue={groupByTagValue}
              groupsData={filteredGroupsData}
              handleGroupByTagValueChange={handleGroupByTagValueChange}
              isLoading={type === 'loading'}
            />
            <PieChart data={filteredGroupsData} />
          </div>
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
    </>
  );
}

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
  const { search } = useLocation();
  const isTagSelected = (tag: string) => tag === groupByTagValue;

  const handleTableRowClick = (value: string) => {
    if (value !== groupByTagValue) {
      handleGroupByTagValueChange(value);
    } else {
      handleGroupByTagValueChange(ALL_TAGS);
    }
  };

  const getSingleViewSearch = () => {
    if (!groupByTagValue || ALL_TAGS) return search;

    const searchParams = new URLSearchParams(search);
    searchParams.delete('query');
    searchParams.set(
      'query',
      appendLabelToQuery(`${appName}{}`, groupByTag, groupByTagValue)
    );
    return `?${searchParams.toString()}`;
  };

  const headRow = [
    // when groupByTag is not selected table represents single "application without tags" group
    {
      name: 'name',
      label: `${groupByTag === '' ? 'App' : 'Tag'} name`,
      sortable: 0,
    },
    { name: 'eventCount', label: 'Event count', sortable: 0 },
    { name: 'avgSamples', label: 'avg samples', sortable: 0 },
    { name: 'stdDeviation', label: 'std deviation samples', sortable: 0 },
  ];

  const bodyRows = groupsData.reduce(
    (acc, { tagName, color, data }): BodyRow[] => {
      const mean = calculateMean(data.samples);
      const row = {
        isRowSelected: isTagSelected(tagName),
        // prevent clicking on single "application without tags" group row
        onClick:
          tagName !== appName ? () => handleTableRowClick(tagName) : undefined,
        cells: [
          {
            value: (
              <div className={styles.tagName}>
                <span
                  className={styles.tagColor}
                  style={{ backgroundColor: color?.toString() }}
                />
                {tagName}
              </div>
            ),
          },
          { value: data.samples.length },
          { value: mean.toFixed(2) },
          { value: calculateStdDeviation(data.samples, mean).toFixed(2) },
        ],
      };
      acc.push(row);

      return acc;
    },
    [] as BodyRow[]
  );
  const table = {
    headRow,
    ...(isLoading
      ? { type: 'not-filled' as const, value: <LoadingSpinner /> }
      : { type: 'filled' as const, bodyRows }),
  };

  return (
    <div className={styles.tableWrapper}>
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
          <TagsSelector
            linkName="Comparison"
            whereDropdownItems={whereDropdownItems}
            groupByTag={groupByTag}
            appName={appName}
          />
          <TagsSelector
            linkName="Diff"
            whereDropdownItems={whereDropdownItems}
            groupByTag={groupByTag}
            appName={appName}
          />
        </div>
      </div>
      <TableUI table={table} className={styles.tagExplorerTable} />
    </div>
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
