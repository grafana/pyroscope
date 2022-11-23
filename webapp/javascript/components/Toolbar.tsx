import React from 'react';
import 'react-dom';

import Spinner from 'react-svg-spinner';

import { useAppSelector } from '@webapp/redux/hooks';
import { selectIsLoadingData } from '@webapp/redux/reducers/continuous';
import { Query } from '@webapp/models/query';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import AppSelector from './AppSelector';

interface ToolbarProps {
  /** callback to be called when an app is selected via the dropdown */
  onSelectedApp: (name: Query) => void;

  filterApp?: React.ComponentProps<typeof AppSelector>['filterApp'];
}
function Toolbar({ onSelectedApp: onSelectedName, filterApp }: ToolbarProps) {
  const isLoadingData = useAppSelector(selectIsLoadingData);

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <AppSelector onSelectedName={onSelectedName} filterApp={filterApp} />
        </div>
        <div className="navbar-space-filler" />
        <div
          className={classNames('spinner-container', {
            visible: isLoadingData,
            loaded: !isLoadingData,
          })}
        >
          {isLoadingData && (
            <Spinner color="rgba(255,255,255,0.6)" size="20px" />
          )}
        </div>
        &nbsp;
        <RefreshButton />
        &nbsp;
        <DateRangePicker />
      </div>
    </>
  );
}

export default Toolbar;
