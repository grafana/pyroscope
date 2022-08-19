import React, { useEffect, useState } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  selectApps,
  reloadApps,
  deleteApp,
} from '@webapp/redux/reducers/settings';
import { addNotification } from '@webapp/redux/reducers/notifications';
import { type App } from '@webapp/models/app';
import Input from '@webapp/ui/Input';
import TableUI from '@webapp/ui/Table';
import cl from 'classnames';

import appsStyles from './Apps.module.css';
import tableStyles from '../SettingsTable.module.scss';
import { getAppTableRows } from './getAppTableRows';

const headRow = [
  { name: '', label: 'Name', sortable: 0 },
  { name: '', label: '', sortable: 0 },
];

function Apps() {
  const dispatch = useAppDispatch();
  const apps = useAppSelector(selectApps);
  const [search, setSearchField] = useState('');
  const [appsInProcessing, setAppsInProcessing] = useState([] as string[]);

  useEffect(() => {
    dispatch(reloadApps());
  }, []);

  const displayApps =
    (apps &&
      apps.filter(
        (x) => x.name.toLowerCase().indexOf(search.toLowerCase()) !== -1
      )) ||
    [];

  const handleDeleteApp = (app: App) => {
    setAppsInProcessing([...appsInProcessing, app.name]);
    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    dispatch(deleteApp(app))
      .unwrap()
      .then(() => {
        setAppsInProcessing(appsInProcessing.filter((x) => x !== app.name));
        // eslint-disable-next-line @typescript-eslint/no-floating-promises
        dispatch(
          addNotification({
            type: 'success',
            title: 'App has been deleted',
            message: `App ${app.name} has been successfully deleted`,
          })
        );
      });
  };

  const tableBodyProps =
    displayApps.length > 0
      ? {
          bodyRows: getAppTableRows(
            displayApps,
            appsInProcessing,
            handleDeleteApp
          ),
          type: 'filled' as const,
        }
      : {
          type: 'not-filled' as const,
          value: 'The list is empty',
          bodyClassName: appsStyles.appsTableEmptyMessage,
        };

  return (
    <>
      <h2>Apps</h2>
      <div className={appsStyles.searchContainer}>
        <Input
          type="text"
          placeholder="Search app"
          value={search}
          onChange={(v) => setSearchField(v.target.value)}
          name="Search app input"
        />
      </div>
      <TableUI
        className={cl(appsStyles.appsTable, tableStyles.settingsTable)}
        table={{ headRow, ...tableBodyProps }}
      />
    </>
  );
}

export default Apps;
