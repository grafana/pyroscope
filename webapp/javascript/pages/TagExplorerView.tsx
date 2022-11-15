import React, { useEffect, useMemo } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import type { Maybe } from 'true-myth';
import type { ClickEvent } from '@szhsin/react-menu';
import Color from 'color';
import TotalSamplesChart from '@webapp/pages/tagExplorer/components/TotalSamplesChart';
import type { Profile } from '@pyroscope/models/src';
import Box, { CollapseBox } from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import TimelineChartWrapper, {
  TimelineGroupData,
} from '@webapp/components/TimelineChart/TimelineChartWrapper';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import TagsSelector from '@webapp/pages/tagExplorer/components/TagsSelector';
import TableUI, { useTableSort, BodyRow } from '@webapp/ui/Table';
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
  setQuery,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';
import PageTitle from '@webapp/components/PageTitle';
import ExploreTooltip from '@webapp/components/TimelineChart/ExploreTooltip';
import { getFormatter } from '@pyroscope/flamegraph/src/format/format';
import { calculateMean, calculateStdDeviation, calculateTotal } from './math';
import { PAGES } from './constants';
import {
  addSpaces,
  getIntegerSpaceLengthForString,
  getTableIntegerSpaceLengthByColumn,
} from './formatTableData';

// eslint-disable-next-line css-modules/no-unused-class
import styles from './TagExplorerView.module.scss';
import { formatTitle } from './formatTitle';

const TIMELINE_SERIES_COLORS = [
  Color.rgb(242, 204, 12),
  Color.rgb(115, 191, 105),
  Color.rgb(138, 184, 255),
  Color.rgb(255, 120, 10),
  Color.rgb(242, 73, 92),
  Color.rgb(87, 148, 242),
  Color.rgb(184, 119, 217),
  Color.rgb(112, 93, 160),
  Color.rgb(55, 135, 45),
  Color.rgb(250, 222, 42),
  Color.rgb(68, 126, 188),
  Color.rgb(193, 92, 23),
  Color.rgb(137, 15, 2),
  Color.rgb(10, 67, 124),
  Color.rgb(109, 31, 98),
  Color.rgb(88, 68, 119),
  Color.rgb(183, 219, 171),
  Color.rgb(244, 213, 152),
  Color.rgb(112, 219, 237),
  Color.rgb(249, 186, 143),
  Color.rgb(242, 145, 145),
  Color.rgb(130, 181, 216),
  Color.rgb(229, 168, 226),
  Color.rgb(174, 162, 224),
  Color.rgb(98, 158, 81),
  Color.rgb(229, 172, 14),
  Color.rgb(100, 176, 200),
  Color.rgb(224, 117, 45),
  Color.rgb(191, 27, 0),
  Color.rgb(10, 80, 161),
  Color.rgb(150, 45, 130),
  Color.rgb(97, 77, 147),
  Color.rgb(154, 196, 138),
  Color.rgb(242, 201, 109),
  Color.rgb(101, 197, 219),
  Color.rgb(249, 147, 78),
  Color.rgb(234, 100, 96),
  Color.rgb(81, 149, 206),
  Color.rgb(214, 131, 206),
  Color.rgb(128, 110, 183),
  Color.rgb(63, 104, 51),
  Color.rgb(150, 115, 2),
  Color.rgb(47, 87, 94),
  Color.rgb(153, 68, 10),
  Color.rgb(88, 20, 12),
  Color.rgb(5, 43, 81),
  Color.rgb(81, 23, 73),
  Color.rgb(63, 43, 91),
  Color.rgb(224, 249, 215),
  Color.rgb(252, 234, 202),
  Color.rgb(207, 250, 255),
  Color.rgb(249, 226, 210),
  Color.rgb(252, 226, 222),
  Color.rgb(186, 223, 244),
  Color.rgb(249, 217, 249),
  Color.rgb(222, 218, 247),
];

const TOP_N_ROWS = 10;
const OTHER_TAG_NAME = 'Other';

// structured data to display/style table cells
interface TableValuesData {
  color?: Color;
  mean: number;
  stdDeviation: number;
  total: number;
  tagName: string;
  totalLabel: string;
  stdDeviationLabel: string;
  meanLabel: string;
}

const formatValue = ({
  value,
  formatter,
  profile,
}: {
  value: number;
  formatter?: ReturnType<typeof getFormatter>;
  profile?: Profile;
}) => {
  const formatterResult =
    formatter && profile
      ? `${formatter.format(value, profile.metadata.sampleRate)}`
      : '0';

  if (String(formatterResult).includes('< 0.01')) {
    return formatter && profile
      ? formatter.formatPrecise(value, profile.metadata.sampleRate)
      : '0';
  }

  return formatterResult;
};

const calculateTableData = ({
  data,
  formatter,
  profile,
}: {
  data: TimelineGroupData[];
  formatter?: ReturnType<typeof getFormatter>;
  profile?: Profile;
}): TableValuesData[] =>
  data.reduce((acc, { tagName, data, color }) => {
    const mean = calculateMean(data.samples);
    const total = calculateTotal(data.samples);
    const stdDeviation = calculateStdDeviation(data.samples, mean);

    acc.push({
      tagName,
      color,
      mean,
      total,
      stdDeviation,
      meanLabel: formatValue({ value: mean, formatter, profile }),
      stdDeviationLabel: formatValue({
        value: stdDeviation,
        formatter,
        profile,
      }),
      totalLabel: formatValue({ value: total, formatter, profile }),
    });

    return acc;
  }, [] as TableValuesData[]);

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
    if (v === OTHER_TAG_NAME) {
      return;
    }

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

  const sortedGroupsByTotal = [...filteredGroupsData].sort(
    (a, b) => calculateTotal(b.data.samples) - calculateTotal(a.data.samples)
  );

  const topNGroups = sortedGroupsByTotal.slice(0, TOP_N_ROWS);
  const groupsRemainder = sortedGroupsByTotal.slice(
    TOP_N_ROWS,
    sortedGroupsByTotal.length
  );

  const groups =
    filteredGroupsData.length > TOP_N_ROWS
      ? [
          ...topNGroups,
          {
            tagName: OTHER_TAG_NAME,
            color: Color('#888'),
            data: {
              samples: groupsRemainder.reduce((acc: number[], current) => {
                return acc.concat(current.data.samples);
              }, []),
            },
          } as TimelineGroupData,
        ]
      : filteredGroupsData;

  return (
    <>
      <PageTitle title={formatTitle('Tag Explorer View', query)} />
      <div className={styles.tagExplorerView} data-testid="tag-explorer-view">
        <Toolbar
          onSelectedApp={(query) => {
            dispatch(setQuery(query));
          }}
        />
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
                timelineGroups={groups}
                // to not "dim" timelines when "All" option is selected
                activeGroup={
                  groupByTagValue !== ALL_TAGS ? groupByTagValue : ''
                }
                showTagsLegend={groups.length > 1}
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
        <CollapseBox
          title={`${appName
            .map((a) => `${a} Tag Breakdown`)
            .unwrapOr('Tag Breakdown')}`}
        >
          <div className={styles.statisticsBox}>
            <div className={styles.pieChartWrapper}>
              {groups?.length ? (
                <TotalSamplesChart filteredGroupsData={groups} />
              ) : (
                <LoadingSpinner />
              )}
            </div>
            <Table
              appName={appName.unwrapOr('')}
              whereDropdownItems={whereDropdownItems}
              groupByTag={groupByTag}
              groupByTagValue={groupByTagValue}
              groupsData={groups}
              handleGroupByTagValueChange={handleGroupByTagValueChange}
              isLoading={type === 'loading'}
              activeTagProfile={activeTagProfile}
            />
          </div>
        </CollapseBox>
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
  activeTagProfile,
}: {
  appName: string;
  whereDropdownItems: string[];
  groupByTag: string;
  groupByTagValue: string | undefined;
  groupsData: TimelineGroupData[];
  isLoading: boolean;
  handleGroupByTagValueChange: (groupedByTagValue: string) => void;
  activeTagProfile?: Profile;
}) {
  const { search } = useLocation();
  const isTagSelected = (tag: string) => tag === groupByTagValue;

  const formatter =
    activeTagProfile &&
    getFormatter(
      activeTagProfile.flamebearer.numTicks,
      activeTagProfile.metadata.sampleRate,
      activeTagProfile.metadata.units
    );

  const handleTableRowClick = (value: string) => {
    // prevent clicking on single "application without tags" group row or Other row
    if (value === appName || value === OTHER_TAG_NAME) {
      return;
    }

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
      label: groupByTag === '' ? 'Application' : 'Tag name',
      sortable: 1,
    },
    { name: 'avgSamples', label: 'Average', sortable: 1 },
    { name: 'stdDeviation', label: 'Standard Deviation', sortable: 1 },
    { name: 'totalSamples', label: 'Total', sortable: 1 },
  ];

  const groupsTotal = useMemo(
    () =>
      groupsData.reduce((acc, current) => {
        return acc + calculateTotal(current.data.samples);
      }, 0),
    [groupsData]
  );

  const tableValuesData = calculateTableData({
    data: groupsData,
    formatter,
    profile: activeTagProfile,
  });

  const tableIntegerSpaceLengthByColumn =
    getTableIntegerSpaceLengthByColumn(tableValuesData);

  const formattedTableData = tableValuesData.map((v) => {
    const meanLength = getIntegerSpaceLengthForString(v.meanLabel);
    const stdDeviationLength = getIntegerSpaceLengthForString(
      v.stdDeviationLabel
    );
    const totalLength = getIntegerSpaceLengthForString(v.totalLabel);

    return {
      ...v,
      totalLabel: addSpaces(
        tableIntegerSpaceLengthByColumn.total,
        totalLength,
        v.totalLabel
      ),
      stdDeviationLabel: addSpaces(
        tableIntegerSpaceLengthByColumn.stdDeviation,
        stdDeviationLength,
        v.stdDeviationLabel
      ),
      meanLabel: addSpaces(
        tableIntegerSpaceLengthByColumn.mean,
        meanLength,
        v.meanLabel
      ),
    };
  });

  const { sortByDirection, sortBy, updateSortParams } = useTableSort(headRow);

  const sortedTableValuesData = (() => {
    const m = sortByDirection === 'asc' ? 1 : -1;
    let sorted: TableValuesData[] = [];

    switch (sortBy) {
      case 'name':
        sorted = formattedTableData.sort(
          (a, b) => m * a.tagName.localeCompare(b.tagName)
        );
        break;
      case 'totalSamples':
        sorted = formattedTableData.sort((a, b) => m * (a.total - b.total));
        break;
      case 'avgSamples':
        sorted = formattedTableData.sort((a, b) => m * (a.mean - b.mean));
        break;
      case 'stdDeviation':
        sorted = formattedTableData.sort(
          (a, b) => m * (a.stdDeviation - b.stdDeviation)
        );
        break;
      default:
        sorted = formattedTableData;
    }

    return sorted;
  })();

  const bodyRows = sortedTableValuesData.reduce(
    (
      acc,
      { tagName, color, total, totalLabel, stdDeviationLabel, meanLabel }
    ): BodyRow[] => {
      const percentage = (total / groupsTotal) * 100;
      const row = {
        isRowSelected: isTagSelected(tagName),
        onClick: () => handleTableRowClick(tagName),
        cells: [
          {
            value: (
              <div className={styles.tagName}>
                <span
                  className={styles.tagColor}
                  style={{ backgroundColor: color?.toString() }}
                />
                <span className={styles.label}>
                  {tagName}
                  <span className={styles.bold}>
                    &nbsp;{`(${percentage.toFixed(2)}%)`}
                  </span>
                </span>
              </div>
            ),
          },
          { value: meanLabel },
          { value: stdDeviationLabel },
          { value: totalLabel },
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
      <TableUI
        updateSortParams={updateSortParams}
        sortBy={sortBy}
        sortByDirection={sortByDirection}
        table={table}
        className={styles.tagExplorerTable}
      />
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
