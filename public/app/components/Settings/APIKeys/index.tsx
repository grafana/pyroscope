import React, { useEffect } from 'react';
import { useHistory } from 'react-router-dom';
import { formatDistance, formatRelative } from 'date-fns/fp';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faPlus } from '@fortawesome/free-solid-svg-icons/faPlus';

import Button from '@pyroscope/ui/Button';
import Icon from '@pyroscope/ui/Icon';
import TableUI, { BodyRow } from '@pyroscope/ui/Table';
import type { APIKey, APIKeys } from '@pyroscope/models/apikeys';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  reloadApiKeys,
  selectAPIKeys,
  deleteAPIKey,
} from '@pyroscope/redux/reducers/settings';
import confirmDelete from '@pyroscope/components/Modals/ConfirmDelete';
import styles from '../SettingsTable.module.scss';

const getBodyRows = (
  keys: APIKeys,
  onDelete: (k: APIKey) => void
): BodyRow[] => {
  const now = new Date();

  const handleDeleteClick = (key: APIKey) => {
    confirmDelete({
      objectType: 'key',
      objectName: key.name,
      onConfirm: () => onDelete(key),
    });
  };

  return keys.reduce((acc, k) => {
    acc.push({
      cells: [
        { value: k.id },
        { value: k.name },
        { value: k.role },
        { value: formatRelative(k.createdAt, now) },
        {
          value: k.expiresAt
            ? `in ${formatDistance(k.expiresAt, now)}`
            : 'never',
          title: k?.expiresAt?.toString(),
        },
        {
          value: (
            <Button
              type="submit"
              kind="danger"
              aria-label="Delete key"
              onClick={() => handleDeleteClick(k)}
            >
              <Icon icon={faTimes} />
            </Button>
          ),
          align: 'center',
        },
      ],
    });
    return acc;
  }, [] as BodyRow[]);
};

const headRow = [
  { name: '', label: 'Id', sortable: 0 },
  { name: '', label: 'Name', sortable: 0 },
  { name: '', label: 'Role', sortable: 0 },
  { name: '', label: 'Creation date', sortable: 0 },
  { name: '', label: 'Expiration date', sortable: 0 },
];

const ApiKeys = () => {
  const dispatch = useAppDispatch();
  const apiKeys = useAppSelector(selectAPIKeys);
  const history = useHistory();

  useEffect(() => {
    dispatch(reloadApiKeys());
  }, [dispatch]);

  const onDelete = (key: APIKey) => {
    dispatch(deleteAPIKey(key))
      .unwrap()
      .then(() => {
        dispatch(reloadApiKeys());
      });
  };

  const tableBodyProps = apiKeys
    ? { type: 'filled' as const, bodyRows: getBodyRows(apiKeys, onDelete) }
    : { type: 'not-filled' as const, value: '' };

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
      <TableUI
        table={{ headRow, ...tableBodyProps }}
        className={styles.settingsTable}
      />
    </>
  );
};

export default ApiKeys;
