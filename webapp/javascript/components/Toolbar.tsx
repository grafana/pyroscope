import React from 'react';
import 'react-dom';

import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { Query, queryToAppName, queryFromAppName } from '@webapp/models/query';
import {
  selectApps,
  reloadAppNames,
  selectQueries,
  selectAppNamesState,
} from '@webapp/redux/reducers/continuous';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import AppSelector from './AppSelector';

interface ToolbarProps {
  /** callback to be called when an app is selected via the dropdown */
  onSelectedApp: (name: Query) => void;

  filterApp?: (names: string) => boolean;
}
function Toolbar({ onSelectedApp, filterApp = () => true }: ToolbarProps) {
  const dispatch = useAppDispatch();
  const appNamesState = useAppSelector(selectAppNamesState);
  const apps = useAppSelector(selectApps).filter((a) => filterApp(a.name));
  const appNames = apps.map((a) => a.name);
  const { query } = useAppSelector(selectQueries);
  const selectedAppName = queryToAppName(query).mapOr('', (q) =>
    appNames.indexOf(q) !== -1 ? q : ''
  );

  const onSelected = (appName: string) => {
    const query = queryFromAppName(appName);
    onSelectedApp(query);
  };

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <AppSelector
            onSelected={onSelected}
            apps={apps}
            selectedAppName={selectedAppName}
            isLoading={appNamesState.type === 'reloading'}
            onRefresh={() => dispatch(reloadAppNames)}
          />
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
