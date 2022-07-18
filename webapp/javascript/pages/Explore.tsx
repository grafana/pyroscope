import React from 'react';

import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import useColorMode from '@webapp/hooks/colorMode.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';

function Explore() {
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();

  return (
    <>
      <Toolbar />
      <Box>
        selectors
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
    </>
  );
}

export default Explore;
