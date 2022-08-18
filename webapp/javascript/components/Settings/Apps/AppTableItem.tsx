import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';
import React from 'react';

import confirmDelete from '@webapp/components/Modals/ConfirmDelete';
import { App } from '@webapp/models/app';
import cx from 'classnames';
import styles from './AppTableItem.module.css';

function DeleteButton(props: ShamefulAny) {
  const { onDelete, app } = props;

  const handleDeleteClick = () => {
    confirmDelete('this app', () => {
      onDelete(app);
    });
  };

  return (
    <Button type="button" kind="danger" onClick={handleDeleteClick}>
      <Icon icon={faTimes} />
    </Button>
  );
}

function AppTableItem(props: ShamefulAny) {
  const { app, onDelete } = props;
  const { name } = app as App;

  return (
    <tr>
      <th>{name}</th>
      <td align="center">
        <div className={styles.actions}>
          <DeleteButton app={app} onDelete={onDelete} />
        </div>
      </td>
    </tr>
  );
}

export default AppTableItem;
