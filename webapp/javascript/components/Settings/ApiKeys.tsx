import React from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';

import { faTimes } from '@fortawesome/free-solid-svg-icons';

import { addDays, addHours, formatDistance, formatRelative } from 'date-fns/fp';
import styles from './SettingsTable.module.css';

const sampleKeys = [
  {
    id: 0,
    key: 'd9cca721a735dac4efe709e0f3518373',
    createdAt: addDays(-5)(new Date()),
    lastAccess: addHours(-2)(new Date()),
  },
  {
    id: 1,
    key: '398fefcb5925a718fd0c812bbeb7e101',
    createdAt: addDays(-50)(new Date()),
    lastAccess: null,
  },
];

const ApiKeys = () => {
  const keys = sampleKeys;
  const now = new Date();
  return (
    <>
      <h2>API keys</h2>
      <table className={styles.settingsTable}>
        <thead>
          <tr>
            <th>Key</th>
            <th>Creation date</th>
            <th>Last access</th>
            <th aria-label="Actions" />
          </tr>
        </thead>
        <tbody>
          {keys.map((key) => (
            <tr key={key.id}>
              <td>{key.key}</td>
              <td>{formatRelative(key.createdAt, now)}</td>
              <td>
                {key.lastAccess
                  ? `${formatDistance(key.lastAccess, now)} ago`
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
