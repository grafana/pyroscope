import React, { useEffect, useState, SetStateAction, Dispatch } from 'react';
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
  fetchTags,
  selectQueries,
  selectAppTags,
  TagsState,
} from '@webapp/redux/reducers/continuous';
import { queryToAppName } from '@webapp/models/query';

import styles from './Explore.module.scss';

function Explore() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();

  // should we save selection while switching views without changing application name ?
  const [selectedTag, setSelectedTag] = useState('');

  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));
  const appName = queryToAppName(query);

  useEffect(() => {
    if (query) {
      dispatch(fetchTags(query));

      setSelectedTag('');
    }
  }, [query]);

  return (
    <div className={styles.explorePage}>
      <Toolbar hideTagsBar={true} />
      <Box>
        <ExploreHeader
          appName={appName}
          tags={tags}
          selectedTag={selectedTag}
          setSelectedTag={setSelectedTag}
        />
        <TimelineChartWrapper
          timezone={offset === 0 ? 'utc' : 'browser'}
          data-testid="timeline-explore-page"
          id="timeline-chart-explore-page"
          timelineA={{ data: undefined }}
          onSelect={() => ({})}
          height="125px"
        />
      </Box>
      <Box>
        <table>
          <thead>
            app name + stats title m+ buttons (explore / single/ comparison/
            diff)
          </thead>
        </table>
      </Box>
      <Box>
        <FlamegraphRenderer
          showCredit={false}
          profile={undefined}
          colorMode={colorMode}
        />
      </Box>
    </div>
  );
}

function ExploreHeader({
  appName,
  tags,
  selectedTag,
  setSelectedTag,
}: {
  appName: Maybe<string>;
  tags: TagsState;
  selectedTag: string;
  setSelectedTag: Dispatch<SetStateAction<string>>;
}) {
  const tagKeys = Object.keys(tags.tags);

  const handleClick = (e: ClickEvent) => {
    setSelectedTag(e.value);
  };

  return (
    <div className={styles.header}>
      <span className={styles.appName}>
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
