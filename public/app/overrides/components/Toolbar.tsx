import React from 'react';
import 'react-dom';

import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { Query } from '@webapp/models/query';
import {
  selectApps,
  reloadAppNames,
  selectQueries,
  selectAppNamesState,
} from '@webapp/redux/reducers/continuous';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import Button from '@webapp/ui/Button';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import DateRangePicker from '@webapp/components/DateRangePicker';
import RefreshButton from '@webapp/components/RefreshButton';
import { AppSelector } from '@phlare/components/AppSelector/AppSelector';
import styles from '@pyroscope/webapp/javascript/components/Toolbar.module.css';

interface ToolbarProps {
  /** callback to be called when an app is selected via the dropdown */
  onSelectedApp: (name: Query) => void;

  filterApp?: (names: string) => boolean;
}
function Toolbar({
  onSelectedApp,
  filterApp: _filterApp = () => true,
}: ToolbarProps) {
  const dispatch = useAppDispatch();
  const appNamesState = useAppSelector(selectAppNamesState);
  const apps = useAppSelector(selectApps);
  const { query } = useAppSelector(selectQueries);
  const selectedQuery = query;

  const onSelected = (query: Query) => {
    onSelectedApp(query);
  };

  const appNamesLoading =
    appNamesState.type === 'reloading' ? (
      <LoadingSpinner className={styles.appNamesLoading} />
    ) : null;

  return (
    <>
      <div className="navbar">
        <div className={styles.leftSide}>
          <AppSelector
            onSelected={onSelected}
            apps={apps}
            selectedQuery={selectedQuery}
          />
          <Button
            aria-label="Refresh Apps"
            icon={faSyncAlt}
            onClick={() => dispatch(reloadAppNames())}
            className={styles.refreshAppsButton}
          />
          {appNamesLoading}
        </div>
        <div className="navbar-space-filler" />
        <RefreshButton />
        &nbsp;
        <DateRangePicker />
      </div>
    </>
  );
}

export default Toolbar;
