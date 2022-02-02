import React, { useEffect } from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import { faTimes } from '@fortawesome/free-solid-svg-icons';

import { formatDistance, formatRelative } from 'date-fns/fp';
import {
  reloadApiKeys,
  selectAPIKeys,
} from '@pyroscope/redux/reducers/settings';
import styles from './SettingsTable.module.css';

const ApiKeys = () => {
  const dispatch = useAppDispatch();
  const apiKeys = useAppSelector(selectAPIKeys);

  useEffect(() => {
    if (typeof apiKeys === 'undefined') {
      dispatch(reloadApiKeys());
    }
  });

  const now = new Date();
  return (
    <>
      <h2>API keys</h2>
      <table className={styles.settingsTable}>
        <thead>
          <tr>
            <th>Name</th>
            <th>Role</th>
            <th>Creation date</th>
            <th>Expiration date</th>
            <th aria-label="Actions" />
          </tr>
        </thead>
        <tbody>
          {apiKeys &&
            apiKeys.map((key) => (
              <tr key={key.id}>
                <td>{key.name}</td>
                <td>{key.role}</td>
                <td>{formatRelative(key.createdAt, now)}</td>
                <td title={key.expiresAt}>
                  {key.expiresAt
                    ? `in ${formatDistance(key.expiresAt, now)}`
                    : 'never'}
                </td>
                <td align="center">
                  <Button type="submit" kind="default" aria-label="Delete key">
                    <Icon icon={faTimes} />
                  </Button>
                </td>
              </tr>
            ))}
        </tbody>
      </table>
    </>
  );
};

export default ApiKeys;
