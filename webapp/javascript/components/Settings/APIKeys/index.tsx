import React, { useEffect } from 'react';
import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faPlus } from '@fortawesome/free-solid-svg-icons/faPlus';
import { useHistory } from 'react-router-dom';

import { formatDistance, formatRelative } from 'date-fns/fp';
import {
  reloadApiKeys,
  selectAPIKeys,
  deleteAPIKey,
} from '@webapp/redux/reducers/settings';
import confirmDelete from '@webapp/components/ConfirmDelete';
import styles from '../SettingsTable.module.css';

const ApiKeys = () => {
  const dispatch = useAppDispatch();
  const apiKeys = useAppSelector(selectAPIKeys);
  const history = useHistory();

  useEffect(() => {
    dispatch(reloadApiKeys());
  }, []);

  const onDelete = (key: ShamefulAny) => {
    dispatch(deleteAPIKey(key))
      .unwrap()
      .then(() => {
        dispatch(reloadApiKeys());
      });
  };

  const handleDeleteClick = (key: ShamefulAny) => {
    confirmDelete('this key', () => {
      onDelete(key);
    });
  };

  const now = new Date();
  return (
    <>
      <h2>API keys</h2>
      <div>
        <Button
          type="submit"
          kind="secondary"
          icon={faPlus}
          onClick={() => history.push('/settings/api-keys/add')}
        >
          Add Key
        </Button>
      </div>
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
                <td title={key?.expiresAt?.toString()}>
                  {key.expiresAt
                    ? `in ${formatDistance(key.expiresAt, now)}`
                    : 'never'}
                </td>
                <td align="center">
                  <Button
                    type="submit"
                    kind="danger"
                    aria-label="Delete key"
                    onClick={() => handleDeleteClick(key)}
                  >
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
