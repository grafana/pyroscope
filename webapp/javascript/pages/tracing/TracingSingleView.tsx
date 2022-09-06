import React, { useEffect, useState } from 'react';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  fetchSingleView,
  selectQueries,
} from '@webapp/redux/reducers/continuous';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { selectionColor } from '@webapp/hooks/timeline.hook';
import Box from '@webapp/ui/Box';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import TimelineTitle from '@webapp/components/TimelineTitle';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/pages/Heatmap/Heatmap';
import { formatTitle } from '../formatTitle';

import styles from './TracingSingleView.module.scss';

function TracingSingleView() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const [selectedTimeRange, setSelectedTimeRange] = useState({
    from: '',
    until: '',
  });

  const { query } = useAppSelector(selectQueries);
  const { singleView, from, until, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  useEffect(() => {
    if (from && until && query && maxNodes) {
      // to get timeline
      // should we make this request to fetch timeline data ?
      // take if from exemplars api request ? (currently return null)
      // move to tracing store ?
      const fetchData = dispatch(fetchSingleView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, maxNodes]);

  const getTimeline = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return {
          data: singleView.timeline,
        };
      }

      default: {
        return {
          data: undefined,
        };
      }
    }
  };

  return (
    <div>
      <PageTitle title={formatTitle('Tracing single', query)} />
      <div className="main-wrapper">
        <Toolbar />
        <Box>
          <TimelineChartWrapper
            timezone={offset === 0 ? 'utc' : 'browser'}
            data-testid="timeline-single"
            id="timeline-chart-single"
            timelineA={getTimeline()}
            onSelect={(from, until) => setSelectedTimeRange({ from, until })}
            height="125px"
            markings={{
              left: {
                from: selectedTimeRange.from || from,
                to: selectedTimeRange.until || until,
                color: selectionColor,
                overlayColor: selectionColor.alpha(0.3),
              },
            }}
            title={<TimelineTitle titleKey="time_range" />}
            selectionType="single"
          />
        </Box>
        <Box>
          <p className={styles.heatmapTitle}>Heatmap</p>
          <Heatmap />
        </Box>
      </div>
    </div>
  );
}

export default TracingSingleView;
