import React from 'react';

import Box from '@webapp/ui/Box';
import PageTitle from '@webapp/components/PageTitle';
import Toolbar from '@webapp/components/Toolbar';
import { useAppSelector } from '@webapp/redux/hooks';
import { selectQueries } from '@webapp/redux/reducers/continuous';

import { formatTitle } from './formatTitle';

import styles from './SandwichView.module.scss';

export default function SandwichView() {
  const { query } = useAppSelector(selectQueries);

  return (
    <>
      <PageTitle title={formatTitle('Sandwich View', query)} />
      <div className={styles.sandwichViewContainer}>
        <Toolbar hideTagsBar />
        <Box>
          <h3>Sandwich view</h3>
          <div className={styles.sandwich}>
            <div className={styles.half}></div>
            <div className={styles.half}></div>
          </div>
        </Box>
      </div>
    </>
  );
}
