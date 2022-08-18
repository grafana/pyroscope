import React from 'react';

import PageTitle from '@webapp/components/PageTitle';
import Toolbar from '@webapp/components/Toolbar';
import { useAppSelector } from '@webapp/redux/hooks';
import useColorMode from '@webapp/hooks/colorMode.hook';
import { selectQueries } from '@webapp/redux/reducers/continuous';
import { formatTitle } from '../formatTitle';
import styles from './TracingView.module.scss';

function TracingView() {
  const { query } = useAppSelector(selectQueries);
  const { colorMode } = useColorMode();

  return (
    <>
      <PageTitle title={formatTitle('Tag Explorer View', query)} />
      <div className={styles.tracingView} data-testid="tag-explorer-view">
        <Toolbar />
        TracingView
      </div>
    </>
  );
}

export default TracingView;
