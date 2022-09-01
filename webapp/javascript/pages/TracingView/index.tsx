import React from 'react';

import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import TimelineTitle from '@webapp/components/TimelineTitle';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import useColorMode from '@webapp/hooks/colorMode.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { selectQueries, setDateRange } from '@webapp/redux/reducers/continuous';
import { formatTitle } from '../formatTitle';
import Heatmap from './Heatmap';
import styles from './TracingView.module.scss';

function TracingView() {
  const { offset } = useTimeZone();
  const { query } = useAppSelector(selectQueries);
  const { colorMode } = useColorMode();
  const dispatch = useAppDispatch();

  return (
    <>
      <PageTitle title={formatTitle('Tracing View', query)} />
      <div className={styles.tracingView} data-testid="tracing-view">
        <Toolbar />
        <Box>
          <div className={styles.timelineWrapper}>
            <TimelineChartWrapper
              selectionType="double"
              timezone={offset === 0 ? 'utc' : 'browser'}
              data-testid="timeline-tracing-view"
              id="timeline-chart-tracing-view"
              timelineA={{ data: undefined }}
              onSelect={(from, until) =>
                dispatch(setDateRange({ from, until }))
              }
              height="125px"
              format="lines"
              title={<TimelineTitle titleKey="samples" />}
            />
          </div>
        </Box>
        <Box>
          <h3 style={{ textAlign: 'center', marginTop: 0 }}>Heat map</h3>
          <Heatmap />
        </Box>
      </div>
    </>
  );
}

export default TracingView;
