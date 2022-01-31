import React, { useEffect, useState } from 'react';
import Button from '@ui/Button';
import Icon from '@ui/Icon';
import { faTimes } from '@fortawesome/free-solid-svg-icons';
import { formatRelative } from 'date-fns';
import { request } from '@pyroscope/services/base';
import { reloadUsers, createUser } from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { type User } from '../../../models/users';
import styles from './UserForm.module.css';

export type UserAddProps = User & { password?: string };

function UserAddForm(props: UserAddProps) {
  const [form, setForm]: [UserAddProps, (value) => void] = useState({});
  const dispatch = useAppDispatch();

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const handleFormSubmit = (event) => {
    const data = {
      ...form,
      role: 'ReadOnly',
      password: btoa(unescape(encodeURIComponent(form.password))),
    };
    dispatch(createUser(data as User));
  };

  return (
    <div className={styles.userForm}>
      <div>
        <label htmlFor="userAddId">ID:</label>{' '}
        <input
          id="userAddId"
          name="id"
          value={form.id}
          onChange={handleFormChange}
          disabled
        />
      </div>
      <div>
        <label htmlFor="userAddName">Name:</label>{' '}
        <input
          id="userAddName"
          name="name"
          value={form.name}
          onChange={handleFormChange}
        />
      </div>
      <div>
        <label htmlFor="userAddEmail">Email:</label>
        <input
          id="userAddEmail"
          name="email"
          value={form.email}
          onChange={handleFormChange}
        />
      </div>
      <div>
        <label htmlFor="userAddFullName">Full Name:</label>
        <input
          id="userAddFullName"
          name="fullName"
          value={form.fullName}
          onChange={handleFormChange}
        />
      </div>
      <div>
        <label htmlFor="userAddPassword">Password: </label>
        <input
          id="userAddPassword"
          name="password"
          onChange={handleFormChange}
        />
      </div>
      <div>
        <button onClick={handleFormSubmit}>Add user</button>
      </div>
    </div>
  );
}

export default UserAddForm;
