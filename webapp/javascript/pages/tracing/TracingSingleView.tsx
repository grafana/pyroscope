import React, { useEffect, useState } from 'react';

import { selectQueries } from '@webapp/redux/reducers/continuous';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import Box from '@webapp/ui/Box';
import Toolbar from '@webapp/components/Toolbar';
import PageTitle from '@webapp/components/PageTitle';
import { Heatmap } from '@webapp/pages/Heatmap/Heatmap';
import { formatTitle } from '../formatTitle';

import styles from './TracingSingleView.module.scss';

function TracingSingleView() {
  const [selectedTimeRange, setSelectedTimeRange] = useState({
    from: '',
    until: '',
  });

  // console.log(from, until );

  // const { query } = useAppSelector(selectQueries);

  return (
    <div>
      {/* <PageTitle title={formatTitle('Tracing single', query)} /> */}
      <div className="main-wrapper">
        <Toolbar />
        <Box>
          <p className={styles.heatmapTitle}>Heatmap</p>
          <Heatmap />
        </Box>
      </div>
    </div>
  );
}

export default TracingSingleView;
