import React from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes, faCheck } from '@fortawesome/free-solid-svg-icons';
import { formatRelative } from 'date-fns';
import cx from 'classnames';
import { type User } from '../../../models/users';
import styles from './UserTableItem.module.css';

function DisableButton(props) {
  const { user, onDisable } = props;
  const icon = user.isDisabled ? faCheck : faTimes;
  return (
    <Button type="button" kind="default" onClick={onDisable}>
      <Icon icon={icon} /> {user.isDisabled ? 'Enable' : 'Disable'}
    </Button>
  );
}

function UserTableItem(props) {
  const { user, onDisable } = props;
  const { id, isDisabled, fullName, role, updatedAt, email, name } =
    user as User;

  return (
    <tr className={cx({ [styles.disabled]: isDisabled })}>
      <td>{id}</td>
      <td>{name}</td>
      <td>{email}</td>
      <td>{fullName}</td>
      <td>{role}</td>
      <td>{formatRelative(updatedAt, new Date())}</td>
      <td align="center">
        {' '}
        <DisableButton user={user} onDisable={onDisable} />
      </td>
    </tr>
  );
}

export default UserTableItem;
