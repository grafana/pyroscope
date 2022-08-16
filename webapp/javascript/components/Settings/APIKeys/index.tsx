import React, { useEffect } from 'react';
import { useHistory } from 'react-router-dom';
import { formatDistance, formatRelative } from 'date-fns/fp';

import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';
import TableUI, { useTable, BodyRow } from '@webapp/ui/Table';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faPlus } from '@fortawesome/free-solid-svg-icons/faPlus';
import type { APIKey, APIKeys } from '@webapp/models/apikeys';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  reloadApiKeys,
  selectAPIKeys,
  deleteAPIKey,
} from '@webapp/redux/reducers/settings';
import confirmDelete from '@webapp/components/Modals/ConfirmDelete';
import styles from '../SettingsTable.module.scss';

const getBodyRows = (keys: APIKeys, onDelete: any): BodyRow[] => {
  const now = new Date();

  const handleDeleteClick = (key: APIKey) => {
    confirmDelete('this key', () => {
      onDelete(key);
    });
  };

  return keys.reduce((acc, k) => {
    acc.push({
      cells: [
        { value: k.name },
        { value: k.id },
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

const ApiKeys = () => {
  const dispatch = useAppDispatch();
  const apiKeys = useAppSelector(selectAPIKeys);
  const history = useHistory();

  useEffect(() => {
    dispatch(reloadApiKeys());
  }, []);

  const onDelete = (key: APIKey) => {
    dispatch(deleteAPIKey(key))
      .unwrap()
      .then(() => {
        dispatch(reloadApiKeys());
      });
  };

  const headRow = [
    { name: '', label: 'Name', sortable: 0 },
    { name: '', label: 'Role', sortable: 0 },
    { name: '', label: 'Creation date', sortable: 0 },
    { name: '', label: 'Expiration date', sortable: 0 },
    { name: '', label: 'Role', sortable: 0, 'aria-label': 'Actions' },
  ];
  // we should skip call for not sortable tables/heads
  // CHECK FOR EVERY common Table component usage
  const tableProps = useTable(headRow);
  // no keys -> no table
  const tableBodyProps = apiKeys
    ? { bodyRows: getBodyRows(apiKeys, onDelete) }
    : { error: { value: '' } };

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
        {...tableProps}
        // todo fix
        // @ts-ignore
        table={{ headRow, ...tableBodyProps }}
        className={styles.settingsTable}
      />
    </>
  );
};

export default ApiKeys;
