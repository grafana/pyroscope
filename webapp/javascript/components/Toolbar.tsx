import React from 'react';
import 'react-dom';

import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { Query } from '@webapp/models/query';
import {
  selectAppNames,
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
  const appNames = useAppSelector(selectAppNames).filter(filterApp);
  const { query } = useAppSelector(selectQueries);

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <AppSelector
            onSelected={onSelectedApp}
            appNames={appNames}
            selectedQuery={query}
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
