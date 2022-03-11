import React, { useState } from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { faToggleOff } from '@fortawesome/free-solid-svg-icons/faToggleOff';
import { faToggleOn } from '@fortawesome/free-solid-svg-icons/faToggleOn';

import { formatRelative } from 'date-fns';
import cx from 'classnames';
import Dropdown, { MenuItem } from '@ui/Dropdown';
import {
  reloadUsers,
  changeUserRole,
} from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import confirmDelete from '../../ConfirmDelete';
import { type User } from '../../../models/users';
import styles from './UserTableItem.module.css';

function DisableButton(props) {
  const { user, onDisable } = props;
  const icon = user.isDisabled ? faToggleOff : faToggleOn;
  return (
    <Button type="button" kind="secondary" onClick={() => onDisable(user)}>
      <Icon icon={icon} onClick={onDisable} />
    </Button>
  );
}

function EditRoleDropdown(props) {
  const { user } = props;
  const { role } = user;
  const dispatch = useAppDispatch();
  const [status, setStatus] = useState(false);

  const handleEdit = (evt) => {
    if (evt.value !== user.role) {
      dispatch(changeUserRole({ id: user.id, role: evt.value }))
        .unwrap()
        .then(() => dispatch(reloadUsers()))
        .then(() => setStatus(true));
    }
  };

  return (
    <div className={styles.role}>
      <Dropdown label={`Role ${role}`} value={role} onItemClick={handleEdit}>
        <MenuItem value="Admin">Admin</MenuItem>
        <MenuItem value="ReadOnly">Readonly</MenuItem>
      </Dropdown>
      {status ? <Icon icon={faCheck} /> : null}
    </div>
  );
}

function DeleteButton(props) {
  const { onDelete, user } = props;

  const handleDeleteClick = () => {
    confirmDelete('this user', () => {
      onDelete(user);
    });
  };

  return (
    <Button type="button" kind="danger" onClick={handleDeleteClick}>
      <Icon icon={faTimes} />
    </Button>
  );
}

function UserTableItem(props) {
  const { user, onDisable, isCurrent, onDelete } = props;
  const { id, isDisabled, fullName, role, updatedAt, email, name } =
    user as User;

  return (
    <tr className={cx({ [styles.disabled]: isDisabled })}>
      <td>{id}</td>
      <td>{name}</td>
      <td>{email}</td>
      <td>{fullName}</td>
      <td>{isCurrent ? role : <EditRoleDropdown user={user} />}</td>
      <td>{formatRelative(new Date(updatedAt), new Date())}</td>
      <td align="center">
        {!isCurrent ? (
          <div className={styles.actions}>
            <DisableButton user={user} onDisable={onDisable} />
            <DeleteButton user={user} onDelete={onDelete} />
          </div>
        ) : null}
      </td>
    </tr>
  );
}

export default UserTableItem;
