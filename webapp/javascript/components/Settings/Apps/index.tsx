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
import AppTableItem from './AppTableItem';

import appsStyles from './Apps.module.css';
import tableStyles from '../SettingsTable.module.css';

function Apps() {
  const dispatch = useAppDispatch();
  const apps = useAppSelector(selectApps);
  const [search, setSearchField] = useState('');

  useEffect(() => {
    dispatch(reloadApps());
  }, []);
  const displayApps =
    (apps &&
      apps.filter(
        (x) =>
          JSON.stringify(x).toLowerCase().indexOf(search.toLowerCase()) !== -1
      )) ||
    [];

  const handleDeleteApp = (app: App) => {
    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    dispatch(deleteApp(app))
      .unwrap()
      .then(() => {
        // eslint-disable-next-line @typescript-eslint/no-floating-promises
        dispatch(
          addNotification({
            type: 'success',
            title: 'App has been deleted',
            message: `App name#${app.name} has been successfully deleted`,
          })
        );
      });
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
      <table
        className={[appsStyles.appsTable, tableStyles.settingsTable].join(' ')}
        data-testid="apps-table"
      >
        <thead>
          <tr>
            <th>Name</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {displayApps.length ? (
            displayApps.map((app) => (
              <AppTableItem
                app={app}
                key={`appTableItem${app.name}`}
                onDelete={handleDeleteApp}
              />
            ))
          ) : (
            <tr>
              <td className={appsStyles.appsTableEmptyMessage} colSpan={7}>
                The list is empty
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </>
  );
}

export default Apps;
