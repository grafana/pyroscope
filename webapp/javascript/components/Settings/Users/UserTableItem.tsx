import React, { useState } from 'react';
import Button from '@webapp/ui/Button';
import Icon from '@webapp/ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons/faTimes';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { faToggleOff } from '@fortawesome/free-solid-svg-icons/faToggleOff';
import { faToggleOn } from '@fortawesome/free-solid-svg-icons/faToggleOn';

import { formatRelative } from 'date-fns';
import cx from 'classnames';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import { reloadUsers, changeUserRole } from '@webapp/redux/reducers/settings';
import { useAppDispatch } from '@webapp/redux/hooks';
import confirmDelete from '@webapp/components/ConfirmDelete';
import { type User } from '@webapp/models/users';
import styles from './UserTableItem.module.css';

function DisableButton(props: ShamefulAny) {
  const { user, onDisable } = props;
  const icon = user.isDisabled ? faToggleOff : faToggleOn;
  return (
    <Button type="button" kind="secondary" onClick={() => onDisable(user)}>
      <Icon icon={icon} onClick={onDisable} />
    </Button>
  );
}

// TODO: type this correctly
function EditRoleDropdown(props: ShamefulAny) {
  const { user } = props;
  const { role } = user;
  const dispatch = useAppDispatch();
  const [status, setStatus] = useState(false);

  const handleEdit = (evt: ShamefulAny) => {
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

function DeleteButton(props: ShamefulAny) {
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

function UserTableItem(props: ShamefulAny) {
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
