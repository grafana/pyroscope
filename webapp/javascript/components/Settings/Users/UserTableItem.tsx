import React from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons';
import { formatRelative } from 'date-fns';
import cx from 'classnames';
import { type User } from '../../../models/users';
import styles from './UserTableItem.module.css';

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
        <Button type="submit" kind="default">
          <Icon icon={faTimes} onClick={onDisable} />
        </Button>
      </td>
    </tr>
  );
}

export default UserTableItem;
